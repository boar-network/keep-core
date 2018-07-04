package tecdsa

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/crypto/bn256/cloudflare"
)

// Generates and validates commitments based on [TC]
// Utilizes group of points on bn256 elliptic curve for calculations.

// [TC] https://github.com/keep-network/keep-core/blob/master/docs/cryptography/trapdoor-commitments.adoc

// Secret - parameters of commitment generation process
type Secret struct {
	// Secret message
	secret *[]byte
	// Decommitment key; used to commitment validation.
	r *big.Int
}

// Commitment - parameters which can be revealed
type Commitment struct {
	// Public key for a specific commitment.
	pubKey *bn256.G2
	// Master public key for the commitment family.
	h *bn256.G2
	// Calculated commitment.
	commitment *bn256.G2
}

// GenerateCommitment generates a Commitment for passed `secret` message.
// Returns:
// `commitmentParams` - Commitment generation process parameters
// `error` - If generation failed
func GenerateCommitment(secret *[]byte) (*Commitment, *Secret, error) {
	// Generate random private and public keys.
	// [TC]: `privKey = (randomFromZ[0, q - 1])`
	// [TC]: `pubKey = g * privKey`
	_, pubKey, err := bn256.RandomG2(rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	// Generate decommitment key.
	// [TC]: `r = (randomFromZ[0, q - 1])`
	r, _, err := bn256.RandomG1(rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	// Generate random point.
	_, h, err := bn256.RandomG2(rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	// Calculate `secret`'s hash and it's `digest`.
	// [TC]: `digest = sha256(secret) mod q`
	hash := hash256BigInt(secret)
	digest := new(big.Int).Mod(hash, bn256.Order)

	// Calculate `he`
	// [TC]: `he = h + g * privKey`
	he := new(bn256.G2).Add(h, pubKey)

	// Calculate `commitment`
	// [TC]: `commitment = g * digest + he * r`
	commitment := new(bn256.G2).Add(new(bn256.G2).ScalarBaseMult(digest), new(bn256.G2).ScalarMult(he, r))

	// [TC]: `return (r, pubKey, h, commitment)`
	return &Commitment{
			pubKey:     pubKey,
			h:          h,
			commitment: commitment,
		}, &Secret{
			secret: secret,
			r:      r,
		}, nil
}

// ValidateCommitment validates received commitment against revealed secret.
func ValidateCommitment(commitment *Commitment, secret *Secret) (bool, error) {
	// Hash `secret` and calculate `digest`.
	// [TC]: `digest = sha256(secret) mod q`
	hash := hash256BigInt(secret.secret)
	digest := new(big.Int).Mod(hash, bn256.Order)

	// Calculate `a`
	// [TC]: `a = g * r`
	a := new(bn256.G1).ScalarBaseMult(secret.r)

	// Calculate `b`
	// [TC]: `b = h + g * privKey`
	b := new(bn256.G2).Add(commitment.h, commitment.pubKey)

	// Calculate `c`
	// [TC]: `c = commitment - g * digest`
	c := new(bn256.G2).Add(commitment.commitment, new(bn256.G2).Neg(new(bn256.G2).ScalarBaseMult(digest)))

	// Get base point `g`
	g := new(bn256.G1).ScalarBaseMult(big.NewInt(1))

	// Compare pairings
	// [TC]: pairing(a, b) == pairing(g, c)
	if bn256.Pair(a, b).String() != bn256.Pair(g, c).String() {
		return false, fmt.Errorf("pairings doesn't match")
	}
	return true, nil
}

// hash256BigInt - calculates 256-bit hash for passed `secret` and converts it
// to `big.Int`.
func hash256BigInt(secret *[]byte) *big.Int {
	hash := sha256.Sum256(*secret)

	return new(big.Int).SetBytes(hash[:])
}
