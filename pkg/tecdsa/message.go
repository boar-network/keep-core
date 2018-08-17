package tecdsa

import (
	"github.com/keep-network/keep-core/pkg/tecdsa/commitment"
	"github.com/keep-network/keep-core/pkg/tecdsa/curve"
	"github.com/keep-network/keep-core/pkg/tecdsa/zkp"
	"github.com/keep-network/paillier"
)

// PublicKeyShareCommitmentMessage is a message payload that carries signer's
// commitment for a public DSA key share the signer generated.
// It's the very first message exchanged between signers during the T-ECDSA
// distributed key generation process. The message is expected to be broadcast
// publicly.
type PublicKeyShareCommitmentMessage struct {
	signerID string

	publicKeyShareCommitment *commitment.TrapdoorCommitment // C_i
}

// KeyShareRevealMessage is a message payload that carries the sender's share of
// public and secret DSA key during T-ECDSA distributed key generation as well
// as proofs of correctness for the shares. Sender's share is encrypted with
// (t, n) Paillier threshold key. The message is expected to be broadcast
// publicly.
type KeyShareRevealMessage struct {
	signerID string

	secretKeyShare *paillier.Cypher // α_i = E(x_i)
	publicKeyShare *curve.Point     // y_i

	publicKeyShareDecommitmentKey *commitment.DecommitmentKey   // D_i
	secretKeyProof                *zkp.DsaPaillierKeyRangeProof // Π_i
}

// isValid checks secret and public key share against zero knowledge range proof
// shipped alongside them as well as commitment generated by the signer in the
// first phase of the key generation process. This function should be called
// for each received KeyShareRevealMessage before it's combined to a final key.
func (msg *KeyShareRevealMessage) isValid(
	publicKeyShareCommitment *commitment.TrapdoorCommitment, // C_i
	zkpParams *zkp.PublicParameters,
) bool {
	commitmentValid := publicKeyShareCommitment.Verify(
		msg.publicKeyShareDecommitmentKey, msg.publicKeyShare.Bytes(),
	)

	zkpValid := msg.secretKeyProof.Verify(
		msg.secretKeyShare, msg.publicKeyShare, zkpParams,
	)

	return commitmentValid && zkpValid
}

// SignRound1Message is a message produced by each signer as a result of
// executing the first round of T-ECDSA signing algorithm.
type SignRound1Message struct {
	signerID string

	secretKeyFactorShareCommitment *commitment.TrapdoorCommitment // C_1i
}

// SignRound2Message is a message produced by each signer as a result of
// executing the second round of T-ECDSA signing algorithm.
type SignRound2Message struct {
	signerID string

	secretKeyFactorShare                *paillier.Cypher            // u_i = E(ρ_i)
	secretKeyMultipleShare              *paillier.Cypher            // v_i = E(ρ_i * x)
	secretKeyFactorShareDecommitmentKey *commitment.DecommitmentKey // D_1i

	secretKeyFactorProof *zkp.DsaPaillierSecretKeyFactorRangeProof // Π_1i
}

// isValid checks secret key random factor share and secret key multiple share
// against the zero knowledge proof shipped alongside them as well as validates
// commitment generated by signer in the first round.
func (msg *SignRound2Message) isValid(
	secretKeyFactorShareCommitment *commitment.TrapdoorCommitment, // C_1i
	dsaSecretKey *paillier.Cypher, // E(x)
	zkpParams *zkp.PublicParameters,
) bool {
	commitmentValid := secretKeyFactorShareCommitment.Verify(
		msg.secretKeyFactorShareDecommitmentKey,
		msg.secretKeyFactorShare.C.Bytes(),
		msg.secretKeyMultipleShare.C.Bytes(),
	)

	zkpValid := msg.secretKeyFactorProof.Verify(
		msg.secretKeyMultipleShare, dsaSecretKey, msg.secretKeyFactorShare, zkpParams,
	)

	return commitmentValid && zkpValid
}

// SignRound3Message is a message produced by each signer as a result of
// executing the third round of T-ECDSA signing algorithm.
type SignRound3Message struct {
	signerID string

	signatureFactorShareCommitment *commitment.TrapdoorCommitment // C_2i
}

// SignRound4Message is a message produced by each signer as a result of
// executing the fourth round of T-ECDSA signing algorithm.
type SignRound4Message struct {
	signerID string

	signatureFactorPublicShare          *curve.Point                // r_i = g^{k_i}
	signatureUnmaskShare                *paillier.Cypher            // w_i = E(k_i * ρ + c_i * q)
	signatureFactorShareDecommitmentKey *commitment.DecommitmentKey // D_2i

	signatureFactorProof *zkp.EcdsaSignatureFactorRangeProof // Π_2i
}

// isValid checks the signature random multiple public share and signature
// unmask share against the zero knowledge proof shipped alongside them. It
// also validates commitment generated by the signer in the third round.
func (msg *SignRound4Message) isValid(
	signatureFactorShareCommitment *commitment.TrapdoorCommitment, // C_2i
	secretKeyFactor *paillier.Cypher, // u = E(ρ)
	zkpParams *zkp.PublicParameters,
) bool {
	commitmentValid := signatureFactorShareCommitment.Verify(
		msg.signatureFactorShareDecommitmentKey,
		msg.signatureFactorPublicShare.Bytes(),
		msg.signatureUnmaskShare.C.Bytes(),
	)

	zkpValid := msg.signatureFactorProof.Verify(
		msg.signatureFactorPublicShare,
		msg.signatureUnmaskShare,
		secretKeyFactor,
		zkpParams,
	)

	return commitmentValid && zkpValid
}

// SignRound5Message is a message produced by each signer as a result of
// executing the fifth round of T-ECDSA signing algorithm.
type SignRound5Message struct {
	signerID string

	signatureUnmaskPartialDecryption *paillier.PartialDecryption // TDec(w)
}

// SignRound6Message is a message produced by each signer as a result of
// executing the sixth round of T-ECDSA signing algorithm.
type SignRound6Message struct {
	signerID string

	signaturePartialDecryption *paillier.PartialDecryption // TDec(σ)
}