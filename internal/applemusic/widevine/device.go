package widevine

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"sync"
)

// device is the Widevine device the CDM is: its private key, and the
// ClientIdentification message that goes into a license request.
type device struct {
	key      *rsa.PrivateKey
	clientID []byte
}

var (
	loadOnce sync.Once
	loaded   *device
	loadErr  error
)

// loadDevice reads the .wvd (version 2): "WVD", the version, then three
// bytes (type, security level, flags), the PKCS#1 key and the client ID, each
// behind a big-endian 16-bit length.
func loadDevice() (*device, error) {
	loadOnce.Do(func() { loaded, loadErr = parseWVD(wvd) })
	return loaded, loadErr
}

func parseWVD(b64 string) (*device, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, err
	}
	bad := errors.New("widevine: bad device")
	if len(raw) < 9 || raw[0] != 0x57 || raw[1] != 0x56 || raw[2] != 0x44 || raw[3] != 2 {
		return nil, bad
	}
	at := 7
	keyLength := int(raw[at])<<8 | int(raw[at+1])
	at += 2
	if at+keyLength+2 > len(raw) {
		return nil, bad
	}
	key, err := x509.ParsePKCS1PrivateKey(raw[at : at+keyLength])
	if err != nil {
		return nil, err
	}
	at += keyLength
	idLength := int(raw[at])<<8 | int(raw[at+1])
	at += 2
	if at+idLength > len(raw) {
		return nil, bad
	}
	return &device{key: key, clientID: raw[at : at+idLength]}, nil
}
