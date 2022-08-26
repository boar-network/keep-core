package tbtc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"time"

	"github.com/keep-network/keep-core/pkg/chain"
	"github.com/keep-network/keep-core/pkg/tecdsa/retry"

	"github.com/keep-network/keep-common/pkg/persistence"
	"github.com/keep-network/keep-core/pkg/generator"
	"github.com/keep-network/keep-core/pkg/internal/testutils"
	"github.com/keep-network/keep-core/pkg/net"
	"github.com/keep-network/keep-core/pkg/protocol/group"
	"github.com/keep-network/keep-core/pkg/tecdsa/dkg"
)

// TODO: Unit tests for `node.go`.
// TODO: Extract the DKG-specific code into a separate file `pkg/tbtc/dkg.go`

// node represents the current state of an ECDSA node.
type node struct {
	chain          Chain
	netProvider    net.Provider
	walletRegistry *walletRegistry
	dkgExecutor    *dkg.Executor
	protocolLatch  *generator.ProtocolLatch
}

func newNode(
	chain Chain,
	netProvider net.Provider,
	persistence persistence.Handle,
	scheduler *generator.Scheduler,
	config Config,
) *node {
	walletRegistry := newWalletRegistry(persistence)

	dkgExecutor := dkg.NewExecutor(
		logger,
		scheduler,
		config.PreParamsPoolSize,
		config.PreParamsGenerationTimeout,
		config.PreParamsGenerationDelay,
		config.PreParamsGenerationConcurrency,
	)

	latch := generator.NewProtocolLatch()
	scheduler.RegisterProtocol(latch)

	return &node{
		chain:          chain,
		netProvider:    netProvider,
		walletRegistry: walletRegistry,
		dkgExecutor:    dkgExecutor,
		protocolLatch:  latch,
	}
}

// joinDKGIfEligible takes a seed value and undergoes the process of the
// distributed key generation if this node's operator proves to be eligible for
// the group generated by that seed. This is an interactive on-chain process,
// and joinDKGIfEligible can block for an extended period of time while it
// completes the on-chain operation.
func (n *node) joinDKGIfEligible(seed *big.Int, startBlockNumber uint64) {
	logger.Infof(
		"checking eligibility for DKG with seed [0x%x]",
		seed,
	)

	selectedSigningGroupOperators, err := n.chain.SelectGroup(seed)
	if err != nil {
		logger.Errorf(
			"failed to select group with seed [0x%x]: [%v]",
			seed,
			err,
		)
		return
	}

	chainConfig := n.chain.GetConfig()

	if len(selectedSigningGroupOperators) > chainConfig.GroupSize {
		logger.Errorf(
			"group size larger than supported: [%v]",
			len(selectedSigningGroupOperators),
		)
		return
	}

	signing := n.chain.Signing()

	_, operatorPublicKey, err := n.chain.OperatorKeyPair()
	if err != nil {
		logger.Errorf("failed to get operator public key: [%v]", err)
		return
	}

	operatorAddress, err := signing.PublicKeyToAddress(operatorPublicKey)
	if err != nil {
		logger.Errorf("failed to get operator address: [%v]", err)
		return
	}

	indexes := make([]uint8, 0)
	for index, operator := range selectedSigningGroupOperators {
		// See if we are amongst those chosen
		if operator == operatorAddress {
			indexes = append(indexes, uint8(index))
		}
	}

	// Create temporary broadcast channel name for DKG using the
	// group selection seed with the protocol name as prefix.
	channelName := fmt.Sprintf("%s-%s", ProtocolName, seed.Text(16))

	if len(indexes) > 0 {
		logger.Infof(
			"joining DKG with seed [0x%x] and controlling [%v] group members",
			seed,
			len(indexes),
		)

		broadcastChannel, err := n.netProvider.BroadcastChannelFor(channelName)
		if err != nil {
			logger.Errorf("failed to get broadcast channel: [%v]", err)
			return
		}

		membershipValidator := group.NewMembershipValidator(
			&testutils.MockLogger{},
			selectedSigningGroupOperators,
			signing,
		)

		err = broadcastChannel.SetFilter(membershipValidator.IsInGroup)
		if err != nil {
			logger.Errorf(
				"could not set filter for channel [%v]: [%v]",
				broadcastChannel.Name(),
				err,
			)
		}

		blockCounter, err := n.chain.BlockCounter()
		if err != nil {
			logger.Errorf("failed to get block counter: [%v]", err)
			return
		}

		for _, index := range indexes {
			// Capture the member index for the goroutine. The group member
			// index should be in range [1, groupSize] so we need to add 1.
			dkgMemberIndex := index + 1

			go func() {
				n.protocolLatch.Lock()
				defer n.protocolLatch.Unlock()

				retryLoop := newDkgRetryLoop(
					seed,
					startBlockNumber,
					dkgMemberIndex,
					selectedSigningGroupOperators,
					chainConfig,
				)

				// TODO: For this client iteration, the retry loop is started
				//       with a 2h timeout. Once the WalletRegistry is
				//       integrated, the stop signal should be generated
				//       by observing the DKG result submission or timeout.
				loopCtx, cancelLoopCtx := context.WithTimeout(
					context.Background(),
					2*time.Hour,
				)
				defer cancelLoopCtx()

				result, executionEndBlock, err := retryLoop.start(
					loopCtx,
					func(attempt *dkgAttemptParams) (*dkg.Result, uint64, error) {
						logger.Infof(
							"[member:%v] starting dkg attempt [%v] "+
								"with [%v] group members (excluded: [%v])",
							dkgMemberIndex,
							attempt.index,
							chainConfig.GroupSize-len(attempt.excludedMembers),
							attempt.excludedMembers,
						)

						// sessionID must be different for each attempt.
						sessionID := fmt.Sprintf(
							"%v-%v",
							seed.Text(16),
							attempt.index,
						)

						result, executionEndBlock, err := n.dkgExecutor.Execute(
							sessionID,
							attempt.startBlock,
							dkgMemberIndex,
							chainConfig.GroupSize,
							chainConfig.DishonestThreshold(),
							attempt.excludedMembers,
							blockCounter,
							broadcastChannel,
							membershipValidator,
						)
						if err != nil {
							logger.Errorf(
								"[member:%v] dkg attempt [%v] "+
									"failed: [%v]",
								dkgMemberIndex,
								attempt.index,
								err,
							)

							return nil, 0, err
						}

						return result, executionEndBlock, nil
					},
				)
				if err != nil {
					logger.Errorf(
						"[member:%v] failed to execute dkg: [%v]",
						dkgMemberIndex,
						err,
					)
					return
				}

				// TODO: Snapshot the key material before doing on-chain result
				//       submission.

				publicationStartBlock := executionEndBlock
				operatingMemberIndexes := result.Group.OperatingMemberIDs()
				dkgResultChannel := make(chan *DKGResultSubmittedEvent)

				dkgResultSubscription := n.chain.OnDKGResultSubmitted(
					func(event *DKGResultSubmittedEvent) {
						dkgResultChannel <- event
					},
				)
				defer dkgResultSubscription.Unsubscribe()

				err = dkg.Publish(
					logger,
					seed.Text(16),
					publicationStartBlock,
					dkgMemberIndex,
					blockCounter,
					broadcastChannel,
					membershipValidator,
					newDkgResultSigner(n.chain),
					newDkgResultSubmitter(n.chain),
					result,
				)
				if err != nil {
					// Result publication failed. It means that either the result
					// this member proposed is not supported by the majority of
					// group members or that the chain interaction failed.
					// In either case, we observe the chain for the result
					// published by any other group member and based on that,
					// we decide whether we should stay in the final group or
					// drop our membership.
					logger.Warningf(
						"[member:%v] DKG result publication process failed [%v]",
						dkgMemberIndex,
						err,
					)

					if operatingMemberIndexes, err = n.decideSigningGroupMemberFate(
						dkgMemberIndex,
						dkgResultChannel,
						publicationStartBlock,
						result,
					); err != nil {
						logger.Errorf(
							"[member:%v] failed to handle DKG result "+
								"publishing failure: [%v]",
							dkgMemberIndex,
							err,
						)
						return
					}
				}

				// Final signing group may differ from the original DKG
				// group outputted by the sortition protocol. One need to
				// determine the final signing group based on the selected
				// group members who behaved correctly during DKG protocol.
				signingGroupOperators, signingGroupMembersIndexes, err :=
					finalSigningGroup(
						selectedSigningGroupOperators,
						operatingMemberIndexes,
						chainConfig,
					)
				if err != nil {
					logger.Errorf(
						"[member:%v] failed to get final signing "+
							"group: [%v]",
						dkgMemberIndex,
						err,
					)
					return
				}

				// Just like the final and original group may differ, the
				// member index used during the DKG protocol may differ
				// from the final signing group member index as well.
				// We need to remap it.
				signingGroupMemberIndex, ok :=
					signingGroupMembersIndexes[dkgMemberIndex]
				if !ok {
					logger.Errorf(
						"[member:%v] failed to get final signing "+
							"group member index",
						dkgMemberIndex,
					)
					return
				}

				signer := newSigner(
					result.PrivateKeyShare.PublicKey(),
					signingGroupOperators,
					signingGroupMemberIndex,
					result.PrivateKeyShare,
				)

				err = n.walletRegistry.registerSigner(signer)
				if err != nil {
					logger.Errorf(
						"[member:%v] failed to register %s: [%v]",
						dkgMemberIndex,
						signer,
						err,
					)
					return
				}

				logger.Infof(
					"[member:%v] registered %s",
					dkgMemberIndex,
					signer,
				)
			}()
		}
	} else {
		logger.Infof("not eligible for DKG with seed [0x%x]", seed)
	}
}

// decideSigningGroupMemberFate decides what the member will do in case it
// failed to publish its DKG result. Member can stay in the group if it supports
// the same group public key as the one registered on-chain and the member is
// not considered as misbehaving by the group.
func (n *node) decideSigningGroupMemberFate(
	memberIndex group.MemberIndex,
	dkgResultChannel chan *DKGResultSubmittedEvent,
	publicationStartBlock uint64,
	result *dkg.Result,
) ([]group.MemberIndex, error) {
	dkgResultEvent, err := n.waitForDkgResultEvent(
		dkgResultChannel,
		publicationStartBlock,
	)
	if err != nil {
		return nil, err
	}

	groupPublicKeyBytes, err := result.GroupPublicKeyBytes()
	if err != nil {
		return nil, err
	}

	// If member doesn't support the same group public key, it could not stay
	// in the group.
	if !bytes.Equal(groupPublicKeyBytes, dkgResultEvent.GroupPublicKeyBytes) {
		return nil, fmt.Errorf(
			"[member:%v] could not stay in the group because "+
				"the member do not support the same group public key",
			memberIndex,
		)
	}

	misbehavedSet := make(map[group.MemberIndex]struct{})
	for _, misbehavedID := range dkgResultEvent.Misbehaved {
		misbehavedSet[misbehavedID] = struct{}{}
	}

	// If member is considered as misbehaved, it could not stay in the group.
	if _, isMisbehaved := misbehavedSet[memberIndex]; isMisbehaved {
		return nil, fmt.Errorf(
			"[member:%v] could not stay in the group because "+
				"the member is considered as misbehaving",
			memberIndex,
		)
	}

	// Construct a new view of the operating members according to the accepted
	// DKG result.
	operatingMemberIndexes := make([]group.MemberIndex, 0)
	for _, memberID := range result.Group.MemberIDs() {
		if _, isMisbehaved := misbehavedSet[memberID]; !isMisbehaved {
			operatingMemberIndexes = append(operatingMemberIndexes, memberID)
		}
	}

	return operatingMemberIndexes, nil
}

// waitForDkgResultEvent waits for the DKG result submission event. It times out
// and returns error if the DKG result event is not emitted on time.
func (n *node) waitForDkgResultEvent(
	dkgResultChannel chan *DKGResultSubmittedEvent,
	publicationStartBlock uint64,
) (*DKGResultSubmittedEvent, error) {
	config := n.chain.GetConfig()

	timeoutBlock := publicationStartBlock + dkg.PrePublicationBlocks() +
		(uint64(config.GroupSize) * config.ResultPublicationBlockStep)

	blockCounter, err := n.chain.BlockCounter()
	if err != nil {
		return nil, err
	}

	timeoutBlockChannel, err := blockCounter.BlockHeightWaiter(timeoutBlock)
	if err != nil {
		return nil, err
	}

	select {
	case dkgResultEvent := <-dkgResultChannel:
		return dkgResultEvent, nil
	case <-timeoutBlockChannel:
		return nil, fmt.Errorf("ECDSA DKG result publication timed out")
	}
}

// finalSigningGroup takes three parameters:
//   - selectedOperators: Contains addresses of all selected operators. Slice
//     length equals to the groupSize. Each element with index N corresponds
//     to the group member with ID N+1.
//   - operatingMembersIndexes: Contains group members indexes that were neither
//     disqualified nor marked as inactive. Slice length is lesser than or equal
//     to the groupSize.
//   - chainConfig: The tBTC chain's configuration
//
// Using those parameters, this function transforms the selectedOperators
// slice into another slice that contains addresses of all operators
// that were neither disqualified nor marked as inactive. This way, the
// resulting slice has only addresses of properly operating operators
// who form the resulting group.
//
// Apart from that, this function returns a map that holds the final signing
// group members indexes that should be used by particular members who behaved
// correctly during the DKG protocol execution. The key of this map is the
// member index used during DKG protocol and the value is the new member
// index that should be used in the context of the final signing group.
//
// Example:
// selectedOperators: [0xAA, 0xBB, 0xCC, 0xDD, 0xEE]
// operatingMembersIndexes: [5, 1, 3]
// signingGroupOperators: [0xAA, 0xCC, 0xEE]
// signingGroupMembersIndexes: [1:1, 3:2, 5:3]
func finalSigningGroup(
	selectedOperators []chain.Address,
	operatingMembersIndexes []group.MemberIndex,
	chainConfig *ChainConfig,
) (
	[]chain.Address,
	map[group.MemberIndex]group.MemberIndex,
	error,
) {
	// TODO: Use `GroupQuorum` parameter instead of `HonestThreshold`
	if len(selectedOperators) != chainConfig.GroupSize ||
		len(operatingMembersIndexes) < chainConfig.HonestThreshold {
		return nil, nil, fmt.Errorf("invalid input parameters")
	}

	sort.Slice(operatingMembersIndexes, func(i, j int) bool {
		return operatingMembersIndexes[i] < operatingMembersIndexes[j]
	})

	signingGroupOperators := make(
		[]chain.Address,
		len(operatingMembersIndexes),
	)
	signingGroupMembersIndexes := make(
		map[group.MemberIndex]group.MemberIndex,
		len(operatingMembersIndexes),
	)

	for i, operatingMemberID := range operatingMembersIndexes {
		signingGroupOperators[i] = selectedOperators[operatingMemberID-1]
		signingGroupMembersIndexes[operatingMemberID] = group.MemberIndex(i + 1)
	}

	return signingGroupOperators, signingGroupMembersIndexes, nil
}

// dkgResultSigner is responsible for signing the DKG result and verification of
// signatures generated by other group members.
type dkgResultSigner struct {
	chain Chain
}

func newDkgResultSigner(chain Chain) *dkgResultSigner {
	return &dkgResultSigner{
		chain: chain,
	}
}

// SignResult signs the provided DKG result. It returns the information
// pertaining to the signing process: public key, signature, result hash.
// TODO: Add unit tests.
func (drs *dkgResultSigner) SignResult(result *dkg.Result) (*dkg.SignedResult, error) {
	resultHash, err := drs.chain.CalculateDKGResultHash(result)
	if err != nil {
		return nil, fmt.Errorf(
			"dkg result hash calculation failed [%w]",
			err,
		)
	}

	signing := drs.chain.Signing()

	signature, err := signing.Sign(resultHash[:])
	if err != nil {
		return nil, fmt.Errorf(
			"dkg result hash signing failed [%w]",
			err,
		)
	}

	return &dkg.SignedResult{
		PublicKey:  signing.PublicKey(),
		Signature:  signature,
		ResultHash: resultHash,
	}, nil
}

// VerifySignature verifies if the signature was generated from the provided
// DKG result has using the provided public key.
// TODO: Add unit tests.
func (drs *dkgResultSigner) VerifySignature(signedResult *dkg.SignedResult) (bool, error) {
	return drs.chain.Signing().VerifyWithPublicKey(
		signedResult.ResultHash[:],
		signedResult.Signature,
		signedResult.PublicKey,
	)
}

// dkgResultSubmitter is responsible for submitting the DKG result to the chain.
type dkgResultSubmitter struct {
	chain Chain
}

func newDkgResultSubmitter(chain Chain) *dkgResultSubmitter {
	return &dkgResultSubmitter{
		chain: chain,
	}
}

// SubmitResult submits the DKG result along with submitting signatures to the
// chain. In the process, it checks if the number of signatures is above
// the required threshold, whether the result was already submitted and waits
// until the member is eligible for DKG result submission.
// TODO: Add unit tests.
func (drs *dkgResultSubmitter) SubmitResult(
	memberIndex group.MemberIndex,
	result *dkg.Result,
	signatures map[group.MemberIndex][]byte,
	startBlockNumber uint64,
) error {
	config := drs.chain.GetConfig()

	// TODO: Compare signatures count to the GroupQuorum parameter
	if len(signatures) < config.HonestThreshold {
		return fmt.Errorf(
			"could not submit result with [%v] signatures for signature "+
				"honest threshold [%v]",
			len(signatures),
			config.HonestThreshold,
		)
	}

	resultSubmittedChan := make(chan uint64)

	subscription := drs.chain.OnDKGResultSubmitted(
		func(event *DKGResultSubmittedEvent) {
			resultSubmittedChan <- event.BlockNumber
		},
	)
	defer subscription.Unsubscribe()

	dkgState, err := drs.chain.GetDKGState()
	if err != nil {
		return fmt.Errorf("could not check DKG state: [%w]", err)
	}

	if dkgState != AwaitingResult {
		// Someone who was ahead of us in the queue submitted the result. Giving up.
		logger.Infof(
			"[member:%v] DKG is no longer awaiting the result; "+
				"aborting DKG result submission",
			memberIndex,
		)
		return nil
	}

	// Wait until the current member is eligible to submit the result.
	submitterEligibleChan, err := drs.setupEligibilityQueue(
		startBlockNumber,
		memberIndex,
	)
	if err != nil {
		return fmt.Errorf("cannot set up eligibility queue: [%w]", err)
	}

	for {
		select {
		case blockNumber := <-submitterEligibleChan:
			// Member becomes eligible to submit the result. Result submission
			// would trigger the sender side of the result submission event
			// listener but also cause the receiver side (this select)
			// termination that will result with a dangling goroutine blocked
			// forever on the `onSubmittedResultChan` channel. This would
			// cause a resource leak. In order to avoid that, we should
			// unsubscribe from the result submission event listener before
			// submitting the result.
			subscription.Unsubscribe()

			publicKeyBytes, err := result.GroupPublicKeyBytes()
			if err != nil {
				return fmt.Errorf("cannot get public key bytes [%w]", err)
			}

			logger.Infof(
				"[member:%v] submitting DKG result with public key [0x%x] and "+
					"[%v] supporting member signatures at block [%v]",
				memberIndex,
				publicKeyBytes,
				len(signatures),
				blockNumber,
			)

			return drs.chain.SubmitDKGResult(
				memberIndex,
				result,
				signatures,
			)
		case blockNumber := <-resultSubmittedChan:
			logger.Infof(
				"[member:%v] leaving; DKG result submitted by other member "+
					"at block [%v]",
				memberIndex,
				blockNumber,
			)
			// A result has been submitted by other member. Leave without
			// publishing the result.
			return nil
		}
	}
}

// setupEligibilityQueue waits until the current member is eligible to
// submit a result to the blockchain. First member is eligible to submit straight
// away, each following member is eligible after pre-defined block step.
//
// TODO: Revisit the setupEligibilityQueue function. The RFC mentions we should
//	     start submitting from a random member, not the first one.
func (drs *dkgResultSubmitter) setupEligibilityQueue(
	startBlockNumber uint64,
	memberIndex group.MemberIndex,
) (<-chan uint64, error) {
	blockWaitTime := (uint64(memberIndex) - 1) *
		drs.chain.GetConfig().ResultPublicationBlockStep

	eligibleBlockHeight := startBlockNumber + blockWaitTime

	logger.Infof(
		"[member:%v] waiting for block [%v] to submit",
		memberIndex,
		eligibleBlockHeight,
	)

	blockCounter, err := drs.chain.BlockCounter()
	if err != nil {
		return nil, fmt.Errorf("could not get block counter [%w]", err)
	}

	waiter, err := blockCounter.BlockHeightWaiter(eligibleBlockHeight)
	if err != nil {
		return nil, fmt.Errorf("block height waiter failure [%w]", err)
	}

	return waiter, err
}

// dkgRetryLoop is a struct that encapsulates the DKG retry logic.
type dkgRetryLoop struct {
	memberIndex          group.MemberIndex
	selectedOperators    chain.Addresses
	inactiveOperatorsSet map[chain.Address]bool

	chainConfig *ChainConfig

	attemptCounter    uint
	attemptStartBlock uint64

	randomRetryCounter uint
	randomRetrySeed    int64

	delayBlocks              uint64
	delayBlocksBumpFrequency uint
	delayBlocksBumpFactor    uint64
}

func newDkgRetryLoop(
	seed *big.Int,
	initialStartBlock uint64,
	memberIndex group.MemberIndex,
	selectedOperators chain.Addresses,
	chainConfig *ChainConfig,
) *dkgRetryLoop {
	// Pre-compute the 8-byte seed that may be needed for the random
	// retry algorithm. Since the original DKG seed passed as parameter
	// can have a variable length, it is safer to take the first 8 bytes
	// of sha256(seed) as the randomRetrySeed.
	seedSha256 := sha256.Sum256(seed.Bytes())
	randomRetrySeed := int64(binary.BigEndian.Uint64(seedSha256[:8]))

	return &dkgRetryLoop{
		memberIndex:              memberIndex,
		selectedOperators:        selectedOperators,
		inactiveOperatorsSet:     make(map[chain.Address]bool),
		chainConfig:              chainConfig,
		attemptCounter:           0,
		attemptStartBlock:        initialStartBlock,
		randomRetryCounter:       0,
		randomRetrySeed:          randomRetrySeed,
		delayBlocks:              5,
		delayBlocksBumpFrequency: 100,
		delayBlocksBumpFactor:    20,
	}
}

// dkgAttemptParams represents parameters of a DKG attempt.
type dkgAttemptParams struct {
	index           uint
	startBlock      uint64
	excludedMembers []group.MemberIndex
}

// dkgAttemptFn represents a function performing a DKG attempt.
type dkgAttemptFn func(*dkgAttemptParams) (*dkg.Result, uint64, error)

// start begins the DKG retry loop using the given DKG attempt function.
// The retry loop terminates when the DKG result is produced or the ctx
// parameter is done, whatever comes first.
func (drl *dkgRetryLoop) start(
	ctx context.Context,
	dkgAttemptFn dkgAttemptFn,
) (*dkg.Result, uint64, error) {
	// All selected operators should be qualified for the first attempt.
	qualifiedOperatorsSet := drl.selectedOperators.Set()

	for {
		drl.attemptCounter++

		// Check the loop stop signal.
		if ctx.Err() != nil {
			return nil, 0, fmt.Errorf(
				"dkg retry loop received stop signal on attempt [%v]",
				drl.attemptCounter,
			)
		}

		// In order to start attempts >1 in the right place, we need to
		// determine how many blocks were taken by previous attempts. We assume
		// the worst case that each attempt failed at the end of the DKG
		// protocol.
		//
		// That said, we need to increment the previous attempt start
		// block by the number of blocks equal to the protocol duration and
		// by some additional delay blocks. We need a small fixed delay in
		// order to mitigate all corner cases where the actual attempt duration
		// was slightly longer than the expected duration determined by the
		// dkg.ProtocolBlocks function.
		//
		// For example, the attempt may fail at
		// the end of the protocol but the error is returned after some time
		// and more blocks than expected are mined in the meantime.
		// Additionally, we want to strongly extend the delay period
		// periodically in order to give some additional time for nodes to
		// recover and re-fill their internal TSS pre-parameters pools.
		if drl.attemptCounter > 1 {
			delayBlocks := drl.delayBlocks
			if drl.attemptCounter%drl.delayBlocksBumpFrequency == 0 {
				delayBlocks *= drl.delayBlocksBumpFactor
			}

			drl.attemptStartBlock = drl.attemptStartBlock +
				dkg.ProtocolBlocks() +
				delayBlocks
		}

		// Exclude all members controlled by the operators that were not
		// qualified for the current attempt.
		excludedMembers := make([]group.MemberIndex, 0)
		attemptSkipped := false
		for i, operator := range drl.selectedOperators {
			if !qualifiedOperatorsSet[operator] {
				memberIndex := group.MemberIndex(i + 1)
				excludedMembers = append(excludedMembers, memberIndex)

				// If the given member was not qualified for the given attempt,
				// mark this attempt as skipped in order to skip the execution
				// and set up the next attempt properly.
				if memberIndex == drl.memberIndex {
					attemptSkipped = true
					break
				}
			}
		}

		var result *dkg.Result
		var executionEndBlock uint64
		var attemptErr error

		if !attemptSkipped {
			result, executionEndBlock, attemptErr = dkgAttemptFn(&dkgAttemptParams{
				index:           drl.attemptCounter,
				startBlock:      drl.attemptStartBlock,
				excludedMembers: excludedMembers,
			})
			if attemptErr != nil {
				var imErr *dkg.InactiveMembersError
				if errors.As(attemptErr, &imErr) {
					for _, memberIndex := range imErr.InactiveMembersIndexes {
						operator := drl.selectedOperators[memberIndex-1]
						drl.inactiveOperatorsSet[operator] = true
					}
				}
			}
		}

		if attemptSkipped || attemptErr != nil {
			var err error
			qualifiedOperatorsSet, err = drl.qualifiedOperatorsSet()
			if err != nil {
				return nil, 0, fmt.Errorf(
					"cannot get qualified operators for attempt [%v]: [%w]",
					drl.attemptCounter+1,
					err,
				)
			}

			continue
		}

		return result, executionEndBlock, nil
	}
}

// qualifiedOperatorsSet returns a set of operators qualified to participate
// in the given DKG attempt.
func (drl *dkgRetryLoop) qualifiedOperatorsSet() (map[chain.Address]bool, error) {
	// If this is one of the first attempts and random retries were not started
	// yet, check if there are known inactive operators. If the group quorum
	// can be maintained, just exclude the members controlled by the inactive
	// operators from the qualified set.
	if drl.attemptCounter <= 5 &&
		drl.randomRetryCounter == 0 &&
		len(drl.inactiveOperatorsSet) > 0 {
		qualifiedOperators := make(chain.Addresses, 0)
		for _, operator := range drl.selectedOperators {
			if !drl.inactiveOperatorsSet[operator] {
				qualifiedOperators = append(qualifiedOperators, operator)
			}
		}

		// If this attempt pushes us below the group quorum we are falling
		// back to the random retry algorithm that excludes specific members
		// from the original group selection result returned by the sortition
		// pool.
		if len(qualifiedOperators) >= drl.chainConfig.GroupQuorum {
			return qualifiedOperators.Set(), nil
		}
	}

	// In any other case, try to make a random retry.
	qualifiedOperators, err := retry.EvaluateRetryParticipantsForKeyGeneration(
		drl.selectedOperators,
		drl.randomRetrySeed,
		drl.randomRetryCounter,
		uint(drl.chainConfig.GroupQuorum),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"random operator selection failed: [%w]",
			err,
		)
	}

	drl.randomRetryCounter++
	return chain.Addresses(qualifiedOperators).Set(), nil
}
