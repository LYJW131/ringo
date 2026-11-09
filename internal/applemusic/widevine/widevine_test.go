package widevine

// The Widevine CDM's cryptography, checked against the standard library's.

import (
	"bytes"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"encoding/hex"
	"testing"
)

func randomBytes(t *testing.T, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestAESCBC(t *testing.T) {
	key, iv := randomBytes(t, 16), randomBytes(t, 16)
	for _, length := range []int{1, 15, 16, 17, 100} {
		plain := randomBytes(t, length)
		pad := aes.BlockSize - length%aes.BlockSize
		padded := append(bytes.Clone(plain), bytes.Repeat([]byte{byte(pad)}, pad)...)
		block, _ := aes.NewCipher(key)
		encrypted := make([]byte, len(padded))
		cipher.NewCBCEncrypter(block, iv).CryptBlocks(encrypted, padded)
		got, err := aesCBCDecrypt(key, iv, encrypted)
		if err != nil || !bytes.Equal(got, plain) {
			t.Fatalf("length %d: got %x, %v, want %x", length, got, err, plain)
		}
	}
}

func TestAESCMAC(t *testing.T) {
	// RFC 4493's examples.
	key, _ := hex.DecodeString("2b7e151628aed2a6abf7158809cf4f3c")
	msg, _ := hex.DecodeString("6bc1bee22e409f96e93d7e117393172aae2d8a571e03ac9c9eb76fac45af8e5130c81c46a35ce411e5fbc1191a0a52eff69f2445df4f9b17ad2b417be66c3710")
	want := map[int]string{
		0:  "bb1d6929e95937287fa37d129b756746",
		16: "070a16b46b4d4144f79bdd9dd04a287c",
		40: "dfa66747de9ae63030ca32611497c827",
		64: "51f0bebf7e3b9d92fc49741779363cfe",
	}
	for length, mac := range want {
		got, err := aesCMAC(key, msg[:length])
		if err != nil || hex.EncodeToString(got) != mac {
			t.Fatalf("length %d: got %x, %v, want %s", length, got, err, mac)
		}
	}
}

// readFields reads a message's fields independently of the package's reader.
func readFields(t *testing.T, b []byte) map[int][]any {
	t.Helper()
	out := map[int][]any{}
	at := 0
	varint := func() uint64 {
		var r uint64
		for s := uint(0); ; s += 7 {
			x := b[at]
			at++
			r |= uint64(x&0x7f) << s
			if x&0x80 == 0 {
				return r
			}
		}
	}
	for at < len(b) {
		tag := varint()
		if tag&7 == 0 {
			out[int(tag>>3)] = append(out[int(tag>>3)], varint())
		} else {
			n := int(varint())
			out[int(tag>>3)] = append(out[int(tag>>3)], b[at:at+n])
			at += n
		}
	}
	return out
}

// lengthField builds a length-delimited field, and small a one-byte varint, with
// no help from the package.
func lengthField(f int, v []byte) []byte {
	out := []byte{byte(f<<3 | 2)}
	n := len(v)
	for {
		b := byte(n & 0x7f)
		n >>= 7
		if n != 0 {
			out = append(out, b|0x80)
			continue
		}
		out = append(out, b)
		break
	}
	return append(out, v...)
}

func small(f int, v byte) []byte { return []byte{byte(f << 3), v} }

func concat(parts ...[]byte) []byte { return bytes.Join(parts, nil) }

func TestProtobufRoundTrip(t *testing.T) {
	msg := encode(field{1, 300}, field{2, []byte("abc")}, field{3, "xyz"}, field{2, []byte{}})
	got, err := decode(msg)
	if err != nil {
		t.Fatal(err)
	}
	if n, ok := got.numberOf(1); !ok || n != 300 {
		t.Fatalf("field 1: %d %v", n, ok)
	}
	if b, ok := got.bytesOf(3); !ok || string(b) != "xyz" {
		t.Fatalf("field 3: %q %v", b, ok)
	}
	if all := got.allBytesOf(2); len(all) != 2 || string(all[0]) != "abc" || len(all[1]) != 0 {
		t.Fatalf("field 2: %q", all)
	}
	if _, ok := got.bytesOf(1); ok {
		t.Fatal("a varint was read as bytes")
	}
	if _, err := decode(msg[:len(msg)-5]); err == nil {
		t.Fatal("a cut-off message decoded")
	}
}

// A license request is signed with RSA-PSS and a license read with RSA-OAEP.
// The test checks the signature with the device's public key and makes the
// license as a server would.
func TestChallengeAndLicense(t *testing.T) {
	dev, err := loadDevice()
	if err != nil {
		t.Fatal(err)
	}
	public := &dev.key.PublicKey

	kid := randomBytes(t, 16)
	session, err := NewSession(kid)
	if err != nil {
		t.Fatal(err)
	}
	// The challenge is a SignedMessage: 1 type, 2 msg, 3 signature.
	signed := readFields(t, session.Challenge)
	if signed[1][0] != uint64(1) {
		t.Fatalf("type %v, want 1 (LICENSE_REQUEST)", signed[1][0])
	}
	request := signed[2][0].([]byte)
	hashed := sha1.Sum(request)
	// The signature carries a random salt, so it's verified, not compared.
	if err := rsa.VerifyPSS(public, crypto.SHA1, hashed[:], signed[3][0].([]byte), &rsa.PSSOptions{SaltLength: 20}); err != nil {
		t.Fatalf("signature: %v", err)
	}
	// The request names the key ID, asks for a new license, as version 2.1.
	req := readFields(t, request)
	if !bytes.Equal(req[1][0].([]byte), dev.clientID) || req[3][0] != uint64(1) || req[6][0] != uint64(21) {
		t.Fatalf("request fields: %v", req)
	}
	pssh := readFields(t, readFields(t, req[2][0].([]byte))[1][0].([]byte))
	if !bytes.Equal(readFields(t, pssh[1][0].([]byte))[2][0].([]byte), kid) || pssh[2][0] != uint64(1) {
		t.Fatalf("pssh fields: %v", pssh)
	}
	if id := pssh[3][0].([]byte); len(id) != 32 || string(id) != string(bytes.ToUpper(id)) {
		t.Fatalf("request ID %q", id)
	}

	// A license as a server would make it: the session key under OAEP, the
	// content key under the key derived from the request.
	sessionKey, contentKey := randomBytes(t, 16), randomBytes(t, 16)
	context := concat([]byte{1}, []byte("ENCRYPTION\x00"), request, []byte{0, 0, 0, 0x80})
	encryptionKey, err := aesCMAC(sessionKey, context)
	if err != nil {
		t.Fatal(err)
	}
	iv := randomBytes(t, 16)
	block, _ := aes.NewCipher(encryptionKey)
	encrypted := make([]byte, 32)
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(encrypted, concat(contentKey, bytes.Repeat([]byte{16}, 16)))
	container := concat(lengthField(1, kid), lengthField(2, iv), lengthField(3, encrypted), small(4, 2))
	wrapped, err := rsa.EncryptOAEP(sha1.New(), rand.Reader, public, sessionKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	license := concat(small(1, 2), lengthField(2, lengthField(3, container)), lengthField(4, wrapped))
	keys, err := session.Keys(license)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0].Type != 2 || !bytes.Equal(keys[0].Key, contentKey) || !bytes.Equal(keys[0].KID, kid) {
		t.Fatalf("keys: %+v, want %x for %x", keys, contentKey, kid)
	}

	// Not a license, and a license without keys.
	if _, err := session.Keys(concat(small(1, 3), lengthField(2, nil), lengthField(4, wrapped))); err == nil {
		t.Fatal("an answer that isn't a license was read")
	}
	if _, err := session.Keys(small(1, 2)); err == nil {
		t.Fatal("a license without keys was read")
	}
}

func TestLongKeyIDKeepsItsLast16Bytes(t *testing.T) {
	kid := randomBytes(t, 32)
	session, err := NewSession(kid)
	if err != nil {
		t.Fatal(err)
	}
	request := readFields(t, readFields(t, session.Challenge)[2][0].([]byte))
	pssh := readFields(t, readFields(t, request[2][0].([]byte))[1][0].([]byte))
	if got := readFields(t, pssh[1][0].([]byte))[2][0].([]byte); !bytes.Equal(got, kid[16:]) {
		t.Fatalf("key ID %x, want %x", got, kid[16:])
	}
}
