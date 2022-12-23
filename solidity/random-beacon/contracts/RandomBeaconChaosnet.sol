// SPDX-License-Identifier: GPL-3.0-only
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

pragma solidity 0.8.17;

import "./api/IRandomBeacon.sol";
import "./libraries/Callback.sol";
import "@openzeppelin/contracts/access/Ownable.sol";

/// @title Keep Random Beacon Chaosnet Stub
/// @notice Keep Random Beacon stub contract that will be used temporarily until
///         the real-world random beacon client implementation is ready.
/// @dev Used for testing purposes only.
contract RandomBeaconChaosnet is IRandomBeacon, Ownable {
    using Callback for Callback.Data;

    /// @notice Relay entry callback gas limit.
    uint256 public constant _callbackGasLimit = 64_000;

    /// @notice Authorized addresses that can request a relay entry.
    mapping(address => bool) public authorizedRequesters;

    /// @notice Arbitrary number used to generate a relay entry. Initially set
    //          to the Euler's number, updated after each relay entry request.
    uint256 internal seed = 271828182845904523536028747135266249;

    Callback.Data internal callback;

    event RequesterAuthorizationUpdated(
        address indexed requester,
        bool isAuthorized
    );

    /// @notice Request relay entry stub function sets a callback contract
    ///         and executes a callback with an arbitrary relay entry number.
    /// @param callbackContract Beacon consumer callback contract - Wallet Registry
    /// @dev    Despite being a stub function, a requester still needs to be
    ///         authorized by the owner.
    function requestRelayEntry(IRandomBeaconConsumer callbackContract)
        external
    {
        require(
            authorizedRequesters[msg.sender],
            "Requester must be authorized"
        );

        callback.setCallbackContract(callbackContract);

        uint256 relayEntry = uint256(keccak256(abi.encodePacked(seed)));

        // Update the seed number so that a different relay entry is produced
        // every time this function is called. Using the same seed would result
        // in the same group of wallet operators being selected in
        // `WalletRegistry`.
        seed++;

        callback.executeCallback(relayEntry, _callbackGasLimit);
    }

    /// @notice Authorizes a requester of the relay entry.
    function setRequesterAuthorization(address requester, bool isAuthorized)
        external
        onlyOwner
    {
        authorizedRequesters[requester] = isAuthorized;

        emit RequesterAuthorizationUpdated(requester, isAuthorized);
    }
}
