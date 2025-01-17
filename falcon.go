// Copyright (C) 2019-2021 Algorand, Inc.
// This file is part of go-algorand
//
// go-algorand is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// go-algorand is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with go-algorand.  If not, see <https://www.gnu.org/licenses/>.

// Package falcon implements a deterministic variant of the Falcon
// signature scheme.
package falcon

// NOTE: cgo go code couldn't compile with the flags: -Wmissing-prototypes and -Wno-unused-paramete

//#cgo CFLAGS:  -Wall -Wextra -Wpedantic -Wredundant-decls -Wshadow -Wvla -Wpointer-arith -Wno-unused-parameter -Wno-overlength-strings  -O3 -fomit-frame-pointer
// #include "falcon.h"
// #include "deterministic.h"
import "C"

import (
	"errors"
	"fmt"
	"runtime"
	"unsafe"
)

// Falcon cgo errors
var (
	ErrKeygenFail  = errors.New("falcon keygen failed")
	ErrSignFail    = errors.New("falcon sign failed")
	ErrVerifyFail  = errors.New("falcon verify failed")
	ErrConvertFail = errors.New("falcon convert to CT failed")
)

const (
	// PublicKeySize is the size of a Falcon public key.
	PublicKeySize = C.FALCON_DET1024_PUBKEY_SIZE
	// PrivateKeySize is the size of a Falcon private key.
	PrivateKeySize = C.FALCON_DET1024_PRIVKEY_SIZE
	// CurrentSaltVersion is the salt version number used to compute signatures.
	// The salt version is incremented when the signing procedure changes (rarely).
	CurrentSaltVersion = C.FALCON_DET1024_CURRENT_SALT_VERSION
	// CTSignatureSize is the max size in bytes of a Falcon signature in CT format
	CTSignatureSize = C.FALCON_DET1024_SIG_CT_SIZE
	// SignatureMaxSize is the max possible size in bytes of a Falcon signature in a compressed format.
	SignatureMaxSize = C.FALCON_DET1024_SIG_COMPRESSED_MAXSIZE
)

// PublicKey represents  a falcon public key
type PublicKey [PublicKeySize]byte

// PrivateKey represents  a falcon private key
type PrivateKey [PrivateKeySize]byte

// CompressedSignature is a deterministic Falcon signature in compressed
// form, which is variable-length.
type CompressedSignature []byte

// CTSignature is a deterministic Falcon signature in constant-time form,
// which is fixed-length.
type CTSignature [CTSignatureSize]byte

// GenerateKey generates a public/private key pair from the given seed.
func GenerateKey(seed []byte) (PublicKey, PrivateKey, error) {
	var rng C.shake256_context

	seedLen := len(seed)
	seedData := (*C.uchar)(C.NULL)
	if seedLen > 0 {
		seedData = (*C.uchar)(&seed[0])
	}
	C.shake256_init_prng_from_seed(&rng, unsafe.Pointer(seedData), C.size_t(seedLen))

	publicKey := PublicKey{}
	privateKey := PrivateKey{}

	r := C.falcon_det1024_keygen(&rng, unsafe.Pointer(&privateKey[0]), unsafe.Pointer(&publicKey[0]))
	if r != 0 {
		return PublicKey{}, PrivateKey{}, fmt.Errorf("error code is %d: %w", int(r), ErrKeygenFail)
	}

	runtime.KeepAlive(seed)
	return publicKey, privateKey, nil
}

// SignCompressed signs the message with privateKey and returns a compressed
// signature, or an error if signing fails (e.g., due to a malformed private key).
func (sk *PrivateKey) SignCompressed(msg []byte) (CompressedSignature, error) {
	msgLen := len(msg)
	cdata := (*C.uchar)(C.NULL)
	if msgLen > 0 {
		cdata = (*C.uchar)(&msg[0])
	}

	var sigLen C.size_t
	var sig [SignatureMaxSize]byte
	r := C.falcon_det1024_sign_compressed(unsafe.Pointer(&sig[0]), &sigLen, unsafe.Pointer(&(*sk)), unsafe.Pointer(cdata), C.size_t(msgLen))
	if r != 0 {
		return nil, fmt.Errorf("error code %d: %w", int(r), ErrSignFail)
	}

	runtime.KeepAlive(msg)
	return sig[:sigLen], nil
}

// ConvertToCT converts a compressed signature to a CT signature.
func (sig *CompressedSignature) ConvertToCT() (CTSignature, error) {
	sigCT := CTSignature{}

	r := C.falcon_det1024_convert_compressed_to_ct(unsafe.Pointer(&sigCT[0]), unsafe.Pointer(&(*sig)[0]), C.size_t(len(*sig)))
	if r != 0 {
		return CTSignature{}, fmt.Errorf("error code %d: %w", int(r), ErrConvertFail)
	}
	return sigCT, nil
}

// Verify reports whether sig is a valid compressed signature of msg under publicKey.
func (pk *PublicKey) Verify(signature CompressedSignature, msg []byte) error {
	msgLen := len(msg)
	msgData := C.NULL
	if msgLen > 0 {
		msgData = unsafe.Pointer(&msg[0])
	}

	sigLen := len(signature)
	sigData := C.NULL
	if sigLen > 0 {
		sigData = unsafe.Pointer(&signature[0])
	}

	r := C.falcon_det1024_verify_compressed(sigData, C.size_t(sigLen), unsafe.Pointer(&(*pk)), msgData, C.size_t(msgLen))
	if r != 0 {
		return fmt.Errorf("error code %d: %w", int(r), ErrVerifyFail)
	}

	runtime.KeepAlive(msg)
	runtime.KeepAlive(signature)
	return nil
}

// VerifyCTSignature reports whether sig is a valid CT signature of msg under publicKey.
func (pk *PublicKey) VerifyCTSignature(signature CTSignature, msg []byte) error {
	data := C.NULL
	if len(msg) > 0 {
		data = unsafe.Pointer(&msg[0])
	}
	r := C.falcon_det1024_verify_ct(unsafe.Pointer(&signature[0]), unsafe.Pointer(&(*pk)), data, C.size_t(len(msg)))
	if r != 0 {
		return fmt.Errorf("error code %d: %w", int(r), ErrVerifyFail)
	}

	runtime.KeepAlive(msg)
	runtime.KeepAlive(signature)
	return nil
}

// SaltVersion returns the salt version number used in the signature.
// The default salt version is 0, if the signature is too short.
func (sig *CompressedSignature) SaltVersion() int {
	if len(*sig) < 2 {
		return 0
	}
	return int((*sig)[1])
}

// SaltVersion returns the salt version number used in the signature.
// The default salt version is 0, if the signature is too short.
func (sig *CTSignature) SaltVersion() int {
	return int(sig[1])
}
