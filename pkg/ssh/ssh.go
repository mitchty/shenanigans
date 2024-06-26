// Instead of using terraform or pulumi modules in golang lets just
// use the crypto library directly to make life easier.
package ssh

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"golang.org/x/crypto/ssh"
	"io/ioutil"
)

// func main() {
// 	savePrivateFileTo := "./id_rsa_test"
// 	savePublicFileTo := "./id_rsa_test.pub"
// 	bitSize := 4096

// 	privateKey, err := generatePrivateKey(bitSize)
// 	if err != nil {
// 		log.Fatal(err.Error())
// 	}

// 	publicKeyBytes, err := generatePublicKey(&privateKey.PublicKey)
// 	if err != nil {
// 		log.Fatal(err.Error())
// 	}

// 	privateKeyBytes := encodePrivateKeyToPEM(privateKey)

// 	err = writeKeyToFile(privateKeyBytes, savePrivateFileTo)
// 	if err != nil {
// 		log.Fatal(err.Error())
// 	}

// 	err = writeKeyToFile([]byte(publicKeyBytes), savePublicFileTo)
// 	if err != nil {
// 		log.Fatal(err.Error())
// 	}
// }

// TODOecdsa not rsa in future. ecdsa/p521 should be fine to use now

// TODOGenerate a private rsa of size size, needs checking of
// inputs/unit tests for everything here
func NewPrivateRsaKey(size int) (*rsa.PrivateKey, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, size)
	if err != nil {
		return nil, err
	}

	err = privateKey.Validate()
	if err != nil {
		return nil, err
	}

	return privateKey, nil
}

// Take ^^^ then reencode der to an ASN.1 pem format that tools like
// openssh can use.
func EncodePrivateKeyToPEM(privateKey *rsa.PrivateKey) []byte {
	keyDer := x509.MarshalPKCS1PrivateKey(privateKey)

	rsaPrivKey := pem.Block{
		Type:    "RSA PRIVATE KEY",
		Headers: nil,
		Bytes:   keyDer,
	}

	keyPem := pem.EncodeToMemory(&rsaPrivKey)

	return keyPem
}

// Convert rsa.PublicKey to something that can be written to a .pub
// file also for openssh usage.
func NewPublicRsaKey(privatekey *rsa.PublicKey) ([]byte, error) {
	pubRsaKey, err := ssh.NewPublicKey(privatekey)
	if err != nil {
		return nil, err
	}

	pubBytes := ssh.MarshalAuthorizedKey(pubRsaKey)

	return pubBytes, nil
}

// Write ssh files is just a dum af wrapper around WriteFile to make
// sure files have permissions openssh won't whine about.
func WriteSshkeyFile(keyBytes []byte, dest string) error {
	err := ioutil.WriteFile(dest, keyBytes, 0600)
	if err != nil {
		return err
	}

	return nil
}
