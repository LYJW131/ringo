// Package widevine is a Widevine CDM, enough of one to ask for a license and
// read the content keys out of it: the L3 test device that
// apple-music-downloader ships, RSA-PSS to sign the license request, RSA-OAEP
// to read the session key out of the license, and AES-CMAC and AES-CBC for
// the keys in it. Everything is in the standard library but the protocol
// buffers and the CMAC, which are written here.
package widevine

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"errors"
	mathrand "math/rand/v2"
	"strings"
	"time"
)

// Key is a key in a license.
type Key struct {
	KID  []byte
	Type int // 2 for a content key
	Key  []byte
}

// Session is a license request for one key ID, and how to read the license
// that answers it.
type Session struct {
	// Challenge is the signed license request to send.
	Challenge []byte

	device  *device
	request []byte
}

// NewSession makes a license request for key ID kid (the last 16 bytes of it,
// if it is longer).
func NewSession(kid []byte) (*Session, error) {
	dev, err := loadDevice()
	if err != nil {
		return nil, err
	}
	if len(kid) > 16 {
		kid = kid[len(kid)-16:]
	}
	// WidevinePsshData: the key ID.
	psshData := encode(field{2, kid})
	now := time.Now()
	requestID := make([]byte, 16)
	if _, err := rand.Read(requestID[:4]); err != nil {
		return nil, err
	}
	binary.LittleEndian.PutUint64(requestID[8:], uint64(now.UnixMilli()))
	// ContentIdentification.WidevinePsshData: the PSSH data, STREAMING, the request ID.
	widevinePssh := encode(
		field{1, psshData},
		field{2, 1},
		field{3, strings.ToUpper(hex.EncodeToString(requestID))},
	)
	contentID := encode(field{1, widevinePssh})
	request := encode(
		field{1, dev.clientID},
		field{2, contentID},
		field{3, 1}, // NEW
		field{4, int(now.Unix())},
		field{6, 21}, // VERSION_2_1
		field{7, int(mathrand.Uint32N(0xffffffff))},
	)
	// RSASSA-PSS with SHA-1, MGF1 and a 20-byte salt.
	hashed := sha1.Sum(request)
	signature, err := rsa.SignPSS(rand.Reader, dev.key, crypto.SHA1, hashed[:], &rsa.PSSOptions{SaltLength: 20})
	if err != nil {
		return nil, err
	}
	return &Session{
		Challenge: encode(
			field{1, 1}, // LICENSE_REQUEST
			field{2, request},
			field{3, signature},
		),
		device:  dev,
		request: request,
	}, nil
}

// Keys reads the keys out of the license that answers the request.
func (s *Session) Keys(license []byte) ([]Key, error) {
	msg, err := decode(license)
	if err != nil {
		return nil, err
	}
	if typ, _ := msg.numberOf(1); typ != 2 {
		return nil, errors.New("Widevine: the answer isn't a license")
	}
	sessionKeyCipher, ok1 := msg.bytesOf(4)
	body, ok2 := msg.bytesOf(2)
	if !ok1 || !ok2 {
		return nil, errors.New("Widevine: the license has no keys")
	}
	// RSAES-OAEP with SHA-1 and MGF1.
	sessionKey, err := rsa.DecryptOAEP(sha1.New(), nil, s.device.key, sessionKeyCipher, nil)
	if err != nil {
		return nil, err
	}
	// The key that encrypts the content keys: CMAC of 1, "ENCRYPTION", 0,
	// the request, and its length in bits (128).
	context := []byte{1}
	context = append(context, "ENCRYPTION"...)
	context = append(context, 0)
	context = append(context, s.request...)
	context = append(context, 0, 0, 0, 0x80)
	encryptionKey, err := aesCMAC(sessionKey, context)
	if err != nil {
		return nil, err
	}
	container, err := decode(body)
	if err != nil {
		return nil, err
	}
	var keys []Key
	for _, raw := range container.allBytesOf(3) {
		k, err := decode(raw)
		if err != nil {
			return nil, err
		}
		kid, _ := k.bytesOf(1)
		typ, _ := k.numberOf(4)
		key := Key{KID: kid, Type: int(typ), Key: []byte{}}
		iv, hasIV := k.bytesOf(2)
		encrypted, hasKey := k.bytesOf(3)
		if hasIV && hasKey {
			if key.Key, err = aesCBCDecrypt(encryptionKey, iv, encrypted); err != nil {
				return nil, err
			}
		}
		keys = append(keys, key)
	}
	return keys, nil
}
