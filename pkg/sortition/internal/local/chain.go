package local

import (
	"fmt"
	"math/big"
	"sync"

	"github.com/keep-network/keep-core/pkg/chain"
)

var errOperatorUnknown = fmt.Errorf("operator not registered for the staking provider")
var errAuthorizationBelowMinimum = fmt.Errorf("authorization below the minimum")
var errOperatorAlreadyRegisteredInPool = fmt.Errorf("operator is already registered in the pool")

type Chain struct {
	operatorAddress chain.Address

	operatorToStakingProvider      map[chain.Address]chain.Address
	operatorToStakingProviderMutex sync.RWMutex

	// Weight of an operator, as set in the Sortition Pool contract.
	sortitionPool      map[chain.Address]*big.Int
	sortitionPoolMutex sync.RWMutex

	// Stake for a staking provider, as set in the Token Staking contract,
	// minus the pending authorization decrease.
	eligibleStake      map[chain.Address]*big.Int
	eligibleStakeMutex sync.RWMutex

	isPoolLocked bool
}

func Connect(operatorAddress chain.Address) *Chain {
	return &Chain{
		operatorAddress:           operatorAddress,
		operatorToStakingProvider: make(map[chain.Address]chain.Address),
		sortitionPool:             make(map[chain.Address]*big.Int),
		eligibleStake:             make(map[chain.Address]*big.Int),
	}
}

// This is a test util function to setup the chain
func (c *Chain) RegisterOperator(stakingProvider chain.Address, operator chain.Address) {
	c.operatorToStakingProviderMutex.Lock()
	defer c.operatorToStakingProviderMutex.Unlock()

	c.operatorToStakingProvider[c.operatorAddress] = stakingProvider
}

// This is a test util function to setup the chain
func (c *Chain) SetEligibleStake(stakingProvider chain.Address, stake *big.Int) {
	c.eligibleStakeMutex.Lock()
	defer c.eligibleStakeMutex.Unlock()

	c.eligibleStake[stakingProvider] = stake
}

func (c *Chain) CurrentOperatorToStakingProvider() (chain.Address, bool, error) {
	c.operatorToStakingProviderMutex.RLock()
	defer c.operatorToStakingProviderMutex.RUnlock()

	stakingProvider, ok := c.operatorToStakingProvider[c.operatorAddress]
	return stakingProvider, ok, nil
}

func (c *Chain) OperatorToStakingProvider(operator chain.Address) (chain.Address, bool, error) {
	c.operatorToStakingProviderMutex.RLock()
	defer c.operatorToStakingProviderMutex.RUnlock()

	stakingProvider, ok := c.operatorToStakingProvider[operator]
	return stakingProvider, ok, nil
}

func (c *Chain) EligibleStake(stakingProvider chain.Address) (*big.Int, error) {
	c.eligibleStakeMutex.RLock()
	defer c.eligibleStakeMutex.RUnlock()

	eligibleStake, ok := c.eligibleStake[stakingProvider]
	if !ok {
		return big.NewInt(0), nil
	}

	return eligibleStake, nil
}

func (c *Chain) IsPoolLocked() (bool, error) {
	return c.isPoolLocked, nil
}

func (c *Chain) IsOperatorInPool() (bool, error) {
	c.sortitionPoolMutex.RLock()
	defer c.sortitionPoolMutex.RUnlock()

	_, ok := c.sortitionPool[c.operatorAddress]

	return ok, nil
}

func (c *Chain) IsOperatorUpToDate() (bool, error) {
	c.sortitionPoolMutex.RLock()
	defer c.sortitionPoolMutex.RUnlock()

	c.eligibleStakeMutex.RLock()
	defer c.eligibleStakeMutex.RUnlock()

	stakingProvider, isRegistered := c.operatorToStakingProvider[c.operatorAddress]
	if !isRegistered {
		return false, errOperatorUnknown
	}

	eligibleStake, hasStake := c.eligibleStake[stakingProvider]
	weight, isInPool := c.sortitionPool[c.operatorAddress]

	if isInPool {
		return weight.Cmp(eligibleStake) == 0, nil
	} else {
		return !hasStake || eligibleStake.Cmp(big.NewInt(0)) == 0, nil
	}
}

func (c *Chain) JoinSortitionPool() error {
	c.operatorToStakingProviderMutex.Lock()
	defer c.operatorToStakingProviderMutex.Unlock()

	c.sortitionPoolMutex.Lock()
	defer c.sortitionPoolMutex.Unlock()

	stakingProvider, ok := c.operatorToStakingProvider[c.operatorAddress]
	if !ok {
		return errOperatorUnknown
	}

	eligibleStake, ok := c.eligibleStake[stakingProvider]
	if !ok || eligibleStake.Cmp(big.NewInt(0)) == 0 {
		return errAuthorizationBelowMinimum
	}

	_, ok = c.sortitionPool[c.operatorAddress]
	if ok {
		return errOperatorAlreadyRegisteredInPool
	}

	c.sortitionPool[c.operatorAddress] = eligibleStake

	return nil
}

func (c *Chain) UpdateOperatorStatus() error {
	c.eligibleStakeMutex.RLock()
	defer c.eligibleStakeMutex.RUnlock()

	c.sortitionPoolMutex.Lock()
	defer c.sortitionPoolMutex.Unlock()

	stakingProvider, isRegistered := c.operatorToStakingProvider[c.operatorAddress]
	if !isRegistered {
		return errOperatorUnknown
	}

	eligibleStake := c.eligibleStake[stakingProvider]
	c.sortitionPool[c.operatorAddress] = eligibleStake

	return nil
}
