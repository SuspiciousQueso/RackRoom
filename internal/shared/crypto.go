package shared

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"errors"
)

func GenKeypair() (pubB64 string, privB64 string, err error) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(pub), base64.StdEncoding.EncodeToString(priv), nil
}

func DecodePubKey(b64 string) (ed25519.PublicKey, error) {
	b, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, err
	}
	if len(b) != ed25519.PublicKeySize {
		return nil, errors.New("invalid public key size")
	}
	return ed25519.PublicKey(b), nil
}

func DecodePrivKey(b64 string) (ed25519.PrivateKey, error) {
	b, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, err
	}
	if len(b) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid private key size")
	}
	return ed25519.PrivateKey(b), nil
}

func BodySHA256(body []byte) string {
	h := sha256.Sum256(body)
	return base64.StdEncoding.EncodeToString(h[:])
}

// signature covers: timestamp + method + path + bodySha
func Sign(priv ed25519.PrivateKey, timestamp, method, path, bodySha string) string {
	msg := []byte(timestamp + "\n" + method + "\n" + path + "\n" + bodySha)
	sig := ed25519.Sign(priv, msg)
	return base64.StdEncoding.EncodeToString(sig)
}

func Verify(pub ed25519.PublicKey, signatureB64, timestamp, method, path, bodySha string) bool {
	sig, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil {
		return false
	}
	msg := []byte(timestamp + "\n" + method + "\n" + path + "\n" + bodySha)
	return ed25519.Verify(pub, msg, sig)
}
