// SPDX-License-Identifier: MIT
//
// ▓▓▌ ▓▓ ▐▓▓ ▓▓▓▓▓▓▓▓▓▓▌▐▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓ ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓ ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▄
// ▓▓▓▓▓▓▓▓▓▓ ▓▓▓▓▓▓▓▓▓▓▌▐▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓ ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓ ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓
//   ▓▓▓▓▓▓    ▓▓▓▓▓▓▓▀    ▐▓▓▓▓▓▓    ▐▓▓▓▓▓   ▓▓▓▓▓▓     ▓▓▓▓▓   ▐▓▓▓▓▓▌   ▐▓▓▓▓▓▓
//   ▓▓▓▓▓▓▄▄▓▓▓▓▓▓▓▀      ▐▓▓▓▓▓▓▄▄▄▄         ▓▓▓▓▓▓▄▄▄▄         ▐▓▓▓▓▓▌   ▐▓▓▓▓▓▓
//   ▓▓▓▓▓▓▓▓▓▓▓▓▓▀        ▐▓▓▓▓▓▓▓▓▓▓         ▓▓▓▓▓▓▓▓▓▓         ▐▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓
//   ▓▓▓▓▓▓▀▀▓▓▓▓▓▓▄       ▐▓▓▓▓▓▓▀▀▀▀         ▓▓▓▓▓▓▀▀▀▀         ▐▓▓▓▓▓▓▓▓▓▓▓▓▓▓▀
//   ▓▓▓▓▓▓   ▀▓▓▓▓▓▓▄     ▐▓▓▓▓▓▓     ▓▓▓▓▓   ▓▓▓▓▓▓     ▓▓▓▓▓   ▐▓▓▓▓▓▌
// ▓▓▓▓▓▓▓▓▓▓ █▓▓▓▓▓▓▓▓▓ ▐▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓ ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓  ▓▓▓▓▓▓▓▓▓▓
// ▓▓▓▓▓▓▓▓▓▓ ▓▓▓▓▓▓▓▓▓▓ ▐▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓ ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓  ▓▓▓▓▓▓▓▓▓▓
//
//                           Trust math, not hardware.

pragma solidity ^0.8.6;

import "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import "@openzeppelin/contracts/token/ERC20/utils/SafeERC20.sol";
import "./BLS.sol";
import "./Groups.sol";
import "../RandomBeacon.sol";

library Relay {
    using SafeERC20 for IERC20;

    struct Request {
        // Request identifier.
        uint64 id;
        // Identifier of group responsible for signing.
        uint64 groupId;
        // Request start block.
        uint128 startBlock;
    }

    struct Data {
        // Total count of all requests.
        uint64 requestCount;
        // Previous entry value.
        bytes previousEntry;
        // Data of current request.
        Request currentRequest;
        // Address of the sortition pool contract.
        ISortitionPool sortitionPool;
        // Address of the T token contract.
        IERC20 tToken;
        // Address of the staking contract.
        IStaking staking;
        // Fee paid by the relay requester.
        uint256 relayRequestFee;
        // The number of blocks it takes for a group member to become
        // eligible to submit the relay entry.
        uint256 relayEntrySubmissionEligibilityDelay;
        // Hard timeout in blocks for a group to submit the relay entry.
        uint256 relayEntryHardTimeout;
        // Slashing amount for not submitting relay entry
        uint256 relayEntrySubmissionFailureSlashingAmount;
    }

    /// @notice Size of a group in the threshold relay.
    uint256 public constant groupSize = 64;

    /// @notice Seed used as the first relay entry value.
    /// It's a G1 point G * PI =
    /// G * 31415926535897932384626433832795028841971693993751058209749445923078164062862
    /// Where G is the generator of G1 abstract cyclic group.
    bytes public constant relaySeed =
        hex"15c30f4b6cf6dbbcbdcc10fe22f54c8170aea44e198139b776d512d8f027319a1b9e8bfaf1383978231ce98e42bafc8129f473fc993cf60ce327f7d223460663";

    event RelayEntryRequested(
        uint256 indexed requestId,
        uint64 groupId,
        bytes previousEntry
    );

    event RelayEntrySubmitted(uint256 indexed requestId, bytes entry);

    event RelayEntryTimedOut(uint256 indexed requestId);

    /// @notice Initializes the very first `previousEntry` with an initial
    ///         `relaySeed` value. Can be performed only once.
    function initSeedEntry(Data storage self) internal {
        require(
            self.previousEntry.length == 0,
            "Seed entry already initialized"
        );
        self.previousEntry = relaySeed;
    }

    /// @notice Initializes the sortitionPool parameter. Can be performed
    ///         only once.
    /// @param _sortitionPool Value of the parameter.
    function initSortitionPool(Data storage self, ISortitionPool _sortitionPool)
        internal
    {
        require(
            address(self.sortitionPool) == address(0),
            "Sortition pool address already set"
        );

        self.sortitionPool = _sortitionPool;
    }

    /// @notice Initializes the tToken parameter. Can be performed only once.
    /// @param _tToken Value of the parameter.
    function initTToken(Data storage self, IERC20 _tToken) internal {
        require(
            address(self.tToken) == address(0),
            "T token address already set"
        );

        self.tToken = _tToken;
    }

    /// @notice Initializes the staking parameter. Can be performed
    ///         only once.
    /// @param _staking Value of the parameter.
    function initStaking(Data storage self, IStaking _staking) internal {
        require(
            address(self.staking) == address(0),
            "Staking address already set"
        );

        self.staking = _staking;
    }

    /// @notice Creates a request to generate a new relay entry, which will
    ///         include a random number (by signing the previous entry's
    ///         random number).
    /// @param groupId Identifier of the group chosen to handle the request.
    /// @param isFeeRequired Flag which determines whether the request fee
    ///        should be required upon request creation.
    function requestEntry(
        Data storage self,
        uint64 groupId,
        bool isFeeRequired
    ) internal {
        require(
            !isRequestInProgress(self),
            "Another relay request in progress"
        );

        if (isFeeRequired) {
            // slither-disable-next-line reentrancy-events
            self.tToken.safeTransferFrom(
                msg.sender,
                address(this),
                self.relayRequestFee
            );
        }

        uint64 currentRequestId = ++self.requestCount;

        self.currentRequest = Request(
            currentRequestId,
            groupId,
            uint128(block.number)
        );

        emit RelayEntryRequested(currentRequestId, groupId, self.previousEntry);
    }

    /// @notice Creates a new relay entry.
    /// @param submitterIndex Index of the entry submitter.
    /// @param entry Group BLS signature over the previous entry.
    /// @param group Group data.
    function submitEntry(
        Data storage self,
        uint256 submitterIndex,
        bytes calldata entry,
        Groups.Group memory group
    ) internal {
        require(isRequestInProgress(self), "No relay request in progress");
        require(!hasRequestTimedOut(self), "Relay request timed out");

        require(
            submitterIndex > 0 && submitterIndex <= groupSize,
            "Invalid submitter index"
        );
        require(
            group.members[submitterIndex - 1] == msg.sender,
            "Unexpected submitter index"
        );

        (
            uint256 firstEligibleIndex,
            uint256 lastEligibleIndex
        ) = getEligibilityRange(self, entry, groupSize);
        require(
            isEligible(
                self,
                submitterIndex,
                firstEligibleIndex,
                lastEligibleIndex
            ),
            "Submitter is not eligible"
        );

        require(
            BLS.verify(group.groupPubKey, self.previousEntry, entry),
            "Invalid entry"
        );

        // Get the list of members addresses which should be punished due to
        // not submitting the entry on their turn.
        address[] memory punishedMembers = getPunishedMembers(
            self,
            submitterIndex,
            firstEligibleIndex,
            group,
            groupSize
        );
        self.sortitionPool.removeOperators(punishedMembers);

        // If the soft timeout has been exceeded apply stake slashing for
        // all group members. Note that `getSlashingFactor` returns the
        // factor multiplied by 1e18 to avoid precision loss. In that case
        // the final result needs to be divided by 1e18.
        uint256 slashingAmount = (getSlashingFactor(self, groupSize) *
            self.relayEntrySubmissionFailureSlashingAmount) / 1e18;
        // slither-disable-next-line reentrancy-events
        self.staking.slash(slashingAmount, group.members);

        self.previousEntry = entry;
        delete self.currentRequest;

        emit RelayEntrySubmitted(self.requestCount, entry);
    }

    /// @notice Set relayRequestFee parameter.
    /// @param newRelayRequestFee New value of the parameter.
    function setRelayRequestFee(Data storage self, uint256 newRelayRequestFee)
        internal
    {
        require(!isRequestInProgress(self), "Relay request in progress");

        self.relayRequestFee = newRelayRequestFee;
    }

    /// @notice Set relayEntrySubmissionEligibilityDelay parameter.
    /// @param newRelayEntrySubmissionEligibilityDelay New value of the parameter.
    function setRelayEntrySubmissionEligibilityDelay(
        Data storage self,
        uint256 newRelayEntrySubmissionEligibilityDelay
    ) internal {
        require(!isRequestInProgress(self), "Relay request in progress");

        self
            .relayEntrySubmissionEligibilityDelay = newRelayEntrySubmissionEligibilityDelay;
    }

    /// @notice Set relayEntryHardTimeout parameter.
    /// @param newRelayEntryHardTimeout New value of the parameter.
    function setRelayEntryHardTimeout(
        Data storage self,
        uint256 newRelayEntryHardTimeout
    ) internal {
        require(!isRequestInProgress(self), "Relay request in progress");

        self.relayEntryHardTimeout = newRelayEntryHardTimeout;
    }

    /// @notice Set relayEntrySubmissionFailureSlashingAmount parameter.
    /// @param newRelayEntrySubmissionFailureSlashingAmount New value of
    ///        the parameter.
    function setRelayEntrySubmissionFailureSlashingAmount(
        Data storage self,
        uint256 newRelayEntrySubmissionFailureSlashingAmount
    ) internal {
        require(!isRequestInProgress(self), "Relay request in progress");

        self
            .relayEntrySubmissionFailureSlashingAmount = newRelayEntrySubmissionFailureSlashingAmount;
    }

    /// @notice Reports a relay entry timeout.
    /// @param group Group data.
    function reportEntryTimeout(Data storage self, Groups.Group memory group)
        internal
    {
        require(hasRequestTimedOut(self), "Relay request did not time out");

        emit RelayEntryTimedOut(self.currentRequest.id);

        self.staking.slash(
            self.relayEntrySubmissionFailureSlashingAmount,
            group.members
        );

        delete self.currentRequest;
    }

    /// @notice Returns whether a relay entry request is currently in progress.
    /// @return True if there is a request in progress. False otherwise.
    function isRequestInProgress(Data storage self)
        internal
        view
        returns (bool)
    {
        return self.currentRequest.id != 0;
    }

    /// @notice Returns whether the current relay request has timed out.
    /// @return True if the request timed out. False otherwise.
    function hasRequestTimedOut(Data storage self)
        internal
        view
        returns (bool)
    {
        uint256 relayEntryTimeout = (groupSize *
            self.relayEntrySubmissionEligibilityDelay) +
            self.relayEntryHardTimeout;

        return
            isRequestInProgress(self) &&
            block.number > self.currentRequest.startBlock + relayEntryTimeout;
    }

    /// @notice Determines the eligibility range for given relay entry basing on
    ///         current block number.
    /// @dev Parameters _entry and _groupSize are passed because the first
    ///      eligible index is computed as `_entry % _groupSize`. This function
    ///      doesn't use the constant `groupSize` directly to facilitate
    ///      testing. Big group sizes in tests make readability worse and
    ///      dramatically increase the time of execution.
    /// @param _entry Entry value for which the eligibility range should be
    ///        determined.
    /// @param _groupSize Group size for which eligibility range should be determined.
    /// @return firstEligibleIndex Index of the first member which is eligible
    ///         to submit the relay entry.
    /// @return lastEligibleIndex Index of the last member which is eligible
    ///         to submit the relay entry.
    function getEligibilityRange(
        Data storage self,
        bytes calldata _entry,
        uint256 _groupSize
    )
        internal
        view
        returns (uint256 firstEligibleIndex, uint256 lastEligibleIndex)
    {
        // Modulo `groupSize` will give indexes in range <0, groupSize-1>
        // We count member indexes from `1` so we need to add `1` to the result.
        firstEligibleIndex = (uint256(keccak256(_entry)) % _groupSize) + 1;

        // Shift is computed by leveraging Solidity integer division which is
        // equivalent to floored division. That gives the desired result.
        // Shift value should be in range <0, groupSize-1> so we must cap
        // it explicitly.
        uint256 shift = (block.number - self.currentRequest.startBlock) /
            self.relayEntrySubmissionEligibilityDelay;
        shift = shift > _groupSize - 1 ? _groupSize - 1 : shift;

        // Last eligible index must be wrapped if their value is bigger than
        // the group size. If wrapping occurs, the lastEligibleIndex is smaller
        // than the firstEligibleIndex. In that case, the eligibility queue
        // can look as follows: 1, 2 (last), 3, 4, 5, 6, 7 (first), 8.
        lastEligibleIndex = firstEligibleIndex + shift;
        lastEligibleIndex = lastEligibleIndex > _groupSize
            ? lastEligibleIndex - _groupSize
            : lastEligibleIndex;

        return (firstEligibleIndex, lastEligibleIndex);
    }

    /// @notice Returns whether the given submitter index is eligible to submit
    ///         a relay entry within given eligibility range.
    /// @param _submitterIndex Index of the submitter whose eligibility is checked.
    /// @param _firstEligibleIndex First index of the given eligibility range.
    /// @param _lastEligibleIndex Last index of the given eligibility range.
    /// @return True if eligible. False otherwise.
    function isEligible(
        /* solhint-disable-next-line no-unused-vars */
        Data storage self,
        uint256 _submitterIndex,
        uint256 _firstEligibleIndex,
        uint256 _lastEligibleIndex
    ) internal view returns (bool) {
        if (_firstEligibleIndex <= _lastEligibleIndex) {
            // First eligible index is equal or smaller than the last.
            // We just need to make sure the submitter index is in range
            // <firstEligibleIndex, lastEligibleIndex>.
            return
                _firstEligibleIndex <= _submitterIndex &&
                _submitterIndex <= _lastEligibleIndex;
        } else {
            // First eligible index is bigger than the last. We need to deal
            // with wrapped range and check whether the submitter index is
            // either in range <1, lastEligibleIndex> or
            // <firstEligibleIndex, groupSize>.
            return
                _submitterIndex <= _lastEligibleIndex ||
                _firstEligibleIndex <= _submitterIndex;
        }
    }

    /// @notice Determines a list of members which should be punished due to
    ///         not submitting a relay entry on their turn. Punished members
    ///         are determined using the eligibility queue and are taken from
    ///         the <firstEligibleIndex, submitterIndex) range. It also handles
    ///         the `submitterIndex < firstEligibleIndex` case and wraps the
    ///         queue accordingly.
    /// @dev This function doesn't use the constant `groupSize` directly and
    ///      use a `_groupSize` parameter instead to facilitate testing.
    ///      Big group sizes in tests make readability worse and dramatically
    ///      increase the time of execution.
    /// @param _submitterIndex Index of the relay entry submitter.
    /// @param _firstEligibleIndex First index of the given eligibility range.
    /// @param _group Group data.
    /// @param _groupSize _groupSize Group size.
    /// @return An array of members addresses which should be punished due to
    ///         not submitting a relay entry on their turn.
    function getPunishedMembers(
        /* solhint-disable-next-line no-unused-vars */
        Data storage self,
        uint256 _submitterIndex,
        uint256 _firstEligibleIndex,
        Groups.Group memory _group,
        uint256 _groupSize
    ) internal view returns (address[] memory) {
        uint256 punishedMembersCount = _submitterIndex >= _firstEligibleIndex
            ? _submitterIndex - _firstEligibleIndex
            : _groupSize - (_firstEligibleIndex - _submitterIndex);

        address[] memory punishedMembers = new address[](punishedMembersCount);

        for (uint256 i = 0; i < punishedMembersCount; i++) {
            uint256 memberIndex = _firstEligibleIndex + i;
            memberIndex = memberIndex > _groupSize
                ? memberIndex - _groupSize
                : memberIndex;

            punishedMembers[i] = _group.members[memberIndex - 1];
        }

        return punishedMembers;
    }

    /// @notice Computes the slashing factor which should be used during
    ///         slashing of the group which exceeded the soft timeout.
    /// @dev This function doesn't use the constant `groupSize` directly and
    ///      use a `_groupSize` parameter instead to facilitate testing.
    ///      Big group sizes in tests make readability worse and dramatically
    ///      increase the time of execution.
    /// @param _groupSize _groupSize Group size.
    /// @return A slashing factor represented as a fraction multiplied by 1e18
    ///         to avoid precision loss. When using this factor during slashing
    ///         amount computations, the final result should be divided by
    ///         1e18 to obtain a proper result. The slashing factor is
    ///         always in range <0, 1e18>.
    function getSlashingFactor(Data storage self, uint256 _groupSize)
        internal
        view
        returns (uint256)
    {
        uint256 softTimeoutBlock = self.currentRequest.startBlock +
            (_groupSize * self.relayEntrySubmissionEligibilityDelay);

        if (block.number > softTimeoutBlock) {
            uint256 submissionDelay = block.number - softTimeoutBlock;
            uint256 slashingFactor = (submissionDelay * 1e18) /
                self.relayEntryHardTimeout;
            return slashingFactor > 1e18 ? 1e18 : slashingFactor;
        }

        return 0;
    }
}
