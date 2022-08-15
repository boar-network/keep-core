package dkg

import (
	"fmt"
	"github.com/keep-network/keep-core/pkg/crypto/ephemeral"
	"github.com/keep-network/keep-core/pkg/protocol/group"
	"github.com/keep-network/keep-core/pkg/tecdsa/dkg/gen/pb"
)

// Marshal converts this ephemeralPublicKeyMessage to a byte array suitable for
// network communication.
func (epkm *ephemeralPublicKeyMessage) Marshal() ([]byte, error) {
	ephemeralPublicKeys, err := marshalPublicKeyMap(epkm.ephemeralPublicKeys)
	if err != nil {
		return nil, err
	}

	return (&pb.EphemeralPublicKeyMessage{
		SenderID:            uint32(epkm.senderID),
		EphemeralPublicKeys: ephemeralPublicKeys,
		SessionID:           epkm.sessionID,
	}).Marshal()
}

// Unmarshal converts a byte array produced by Marshal to
// an ephemeralPublicKeyMessage
func (epkm *ephemeralPublicKeyMessage) Unmarshal(bytes []byte) error {
	pbMsg := pb.EphemeralPublicKeyMessage{}
	if err := pbMsg.Unmarshal(bytes); err != nil {
		return err
	}

	if err := validateMemberIndex(pbMsg.SenderID); err != nil {
		return err
	}
	epkm.senderID = group.MemberIndex(pbMsg.SenderID)

	ephemeralPublicKeys, err := unmarshalPublicKeyMap(pbMsg.EphemeralPublicKeys)
	if err != nil {
		return err
	}

	epkm.ephemeralPublicKeys = ephemeralPublicKeys
	epkm.sessionID = pbMsg.SessionID

	return nil
}

// Marshal converts this tssRoundOneMessage to a byte array suitable for
// network communication.
func (trom *tssRoundOneMessage) Marshal() ([]byte, error) {
	return (&pb.TSSRoundOneMessage{
		SenderID:  uint32(trom.senderID),
		Payload:   trom.payload,
		SessionID: trom.sessionID,
	}).Marshal()
}

// Unmarshal converts a byte array produced by Marshal to an tssRoundOneMessage.
func (trom *tssRoundOneMessage) Unmarshal(bytes []byte) error {
	pbMsg := pb.TSSRoundOneMessage{}
	if err := pbMsg.Unmarshal(bytes); err != nil {
		return err
	}

	if err := validateMemberIndex(pbMsg.SenderID); err != nil {
		return err
	}

	trom.senderID = group.MemberIndex(pbMsg.SenderID)
	trom.payload = pbMsg.Payload
	trom.sessionID = pbMsg.SessionID

	return nil
}

// Marshal converts this tssRoundTwoMessage to a byte array suitable for
// network communication.
func (trtm *tssRoundTwoMessage) Marshal() ([]byte, error) {
	peersPayload := make(map[uint32][]byte, len(trtm.peersPayload))
	for receiverID, payload := range trtm.peersPayload {
		peersPayload[uint32(receiverID)] = payload
	}

	return (&pb.TSSRoundTwoMessage{
		SenderID:         uint32(trtm.senderID),
		BroadcastPayload: trtm.broadcastPayload,
		PeersPayload:     peersPayload,
		SessionID:        trtm.sessionID,
	}).Marshal()
}

// Unmarshal converts a byte array produced by Marshal to an tssRoundTwoMessage.
func (trtm *tssRoundTwoMessage) Unmarshal(bytes []byte) error {
	pbMsg := pb.TSSRoundTwoMessage{}
	if err := pbMsg.Unmarshal(bytes); err != nil {
		return err
	}

	if err := validateMemberIndex(pbMsg.SenderID); err != nil {
		return err
	}

	peersPayload := make(map[group.MemberIndex][]byte, len(pbMsg.PeersPayload))
	for receiverID, payload := range pbMsg.PeersPayload {
		if err := validateMemberIndex(receiverID); err != nil {
			return err
		}

		peersPayload[group.MemberIndex(receiverID)] = payload
	}

	trtm.senderID = group.MemberIndex(pbMsg.SenderID)
	trtm.broadcastPayload = pbMsg.BroadcastPayload
	trtm.peersPayload = peersPayload
	trtm.sessionID = pbMsg.SessionID

	return nil
}

// Marshal converts this tssRoundThreeMessage to a byte array suitable for
// network communication.
func (trtm *tssRoundThreeMessage) Marshal() ([]byte, error) {
	return (&pb.TSSRoundThreeMessage{
		SenderID:  uint32(trtm.senderID),
		Payload:   trtm.payload,
		SessionID: trtm.sessionID,
	}).Marshal()
}

// Unmarshal converts a byte array produced by Marshal to an tssRoundThreeMessage.
func (trtm *tssRoundThreeMessage) Unmarshal(bytes []byte) error {
	pbMsg := pb.TSSRoundThreeMessage{}
	if err := pbMsg.Unmarshal(bytes); err != nil {
		return err
	}

	if err := validateMemberIndex(pbMsg.SenderID); err != nil {
		return err
	}

	trtm.senderID = group.MemberIndex(pbMsg.SenderID)
	trtm.payload = pbMsg.Payload
	trtm.sessionID = pbMsg.SessionID

	return nil
}

func validateMemberIndex(protoIndex uint32) error {
	// Protobuf does not have uint8 type, so we are using uint32. When
	// unmarshalling message, we need to make sure we do not overflow.
	if protoIndex > group.MaxMemberIndex {
		return fmt.Errorf("invalid member index value: [%v]", protoIndex)
	}
	return nil
}

func marshalPublicKeyMap(
	publicKeys map[group.MemberIndex]*ephemeral.PublicKey,
) (map[uint32][]byte, error) {
	marshalled := make(map[uint32][]byte, len(publicKeys))
	for id, publicKey := range publicKeys {
		if publicKey == nil {
			return nil, fmt.Errorf("nil public key for member [%v]", id)
		}

		marshalled[uint32(id)] = publicKey.Marshal()
	}
	return marshalled, nil
}

func unmarshalPublicKeyMap(
	publicKeys map[uint32][]byte,
) (map[group.MemberIndex]*ephemeral.PublicKey, error) {
	var unmarshalled = make(map[group.MemberIndex]*ephemeral.PublicKey, len(publicKeys))
	for memberID, publicKeyBytes := range publicKeys {
		if err := validateMemberIndex(memberID); err != nil {
			return nil, err
		}

		publicKey, err := ephemeral.UnmarshalPublicKey(publicKeyBytes)
		if err != nil {
			return nil, fmt.Errorf("could not unmarshal public key [%v]", err)
		}

		unmarshalled[group.MemberIndex(memberID)] = publicKey

	}

	return unmarshalled, nil
}
