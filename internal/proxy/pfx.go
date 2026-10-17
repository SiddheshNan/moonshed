package proxy

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"software.sslmate.com/src/go-pkcs12"
)

// PFXToPEM converts a .pfx/.p12 bundle (legacy or AES-encrypted) into a PEM chain and key.
func PFXToPEM(data []byte, password string) (certPEM, keyPEM []byte, err error) {
	key, cert, chain, err := pkcs12.DecodeChain(data, password)
	if err != nil {
		return nil, nil, err
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("unsupported private key: %w", err)
	}
	var certs bytes.Buffer
	for _, c := range append([]*x509.Certificate{cert}, chain...) {
		pem.Encode(&certs, &pem.Block{Type: "CERTIFICATE", Bytes: c.Raw})
	}
	return certs.Bytes(), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}
