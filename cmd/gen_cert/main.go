package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

func main() {
	outDir := "certs"
	_ = os.MkdirAll(outDir, 0755)

	// 1) Root CA
	caCertPath := filepath.Join(outDir, "gotov_ca.pem")
	caKeyPath := filepath.Join(outDir, "gotov_ca_key.pem")

	fmt.Println("🔹 Genererer rot-CA ...")
	caCert, caKey, err := generateCA("goTOV Root CA", caCertPath, caKeyPath)
	if err != nil {
		panic(err)
	}
	fmt.Printf("✅ CA ferdig: %s / %s\n", caCertPath, caKeyPath)

	// 2) Client cert signed by CA (with SAN + Application URI)
	clientCertPath := filepath.Join(outDir, "gotov_cert.pem")
	clientKeyPath := filepath.Join(outDir, "gotov_key.pem")

	fmt.Println("🔹 Genererer klientsertifikat ...")
	err = generateClientCert(
		"goTOV OPC UA Client",
		"urn:bryggeriplc:goTOV",                 // Application URI
		[]string{"bryggeriplc"},                 // DNS SANs
		[]net.IP{net.ParseIP("192.168.10.150")}, // IP SANs
		clientCertPath, clientKeyPath, caCert, caKey,
	)
	if err != nil {
		panic(err)
	}
	fmt.Printf("✅ Klient ferdig: %s / %s\n", clientCertPath, clientKeyPath)

	// 3) Export DER for TwinCAT
	pemToDer(clientCertPath, filepath.Join(outDir, "gotov_cert.der"))
	pemToDer(caCertPath, filepath.Join(outDir, "gotov_ca.der"))
	fmt.Println("✅ Konvertert til DER (.der) for Beckhoff bruk")

	fmt.Println("\n📦 Kopier til PLC:")
	fmt.Println("  - gotov_cert.der →  \\TwinCAT\\Functions\\TF6100-OPC-UA\\Server\\PKI\\CA\\rejected\\certs\\")
	fmt.Println("  - (valgfr.) gotov_ca.der → \\TwinCAT\\Functions\\TF6100-OPC-UA\\Server\\PKI\\CA\\trusted\\certs\\")
}

// -------------------------------------------------------------------

func generateCA(commonName, certPath, keyPath string) (*x509.Certificate, *rsa.PrivateKey, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	ca := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: []string{"goTOV"},
			Country:      []string{"NO"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
		SignatureAlgorithm:    x509.SHA256WithRSA,
	}

	// Subject Key Identifier (RFC 5280)
	pubASN1, _ := asn1.Marshal(key.PublicKey)
	skid := sha1.Sum(pubASN1)
	ca.SubjectKeyId = skid[:]

	certDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}

	savePEM(certPath, "CERTIFICATE", certDER)
	savePEM(keyPath, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(key))
	return ca, key, nil
}

// -------------------------------------------------------------------

func generateClientCert(commonName, appURI string, dnsNames []string, ipAddrs []net.IP,
	certPath, keyPath string, caCert *x509.Certificate, caKey *rsa.PrivateKey) error {

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	u, err := url.Parse(appURI)
	if err != nil {
		return fmt.Errorf("parse application URI: %w", err)
	}

	client := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: commonName, Organization: []string{"goTOV"}},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(5, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		SignatureAlgorithm:    x509.SHA256WithRSA,
		BasicConstraintsValid: true,
		IsCA:                  false,

		// SANs + Application URI (OPC UA expects this)
		DNSNames:    dnsNames,
		IPAddresses: ipAddrs,
		URIs:        []*url.URL{u},

		// Proper chain linking (Authority/Subject Key IDs)
		AuthorityKeyId: caCert.SubjectKeyId,
	}

	// Subject Key Identifier for client
	pubASN1, _ := asn1.Marshal(key.PublicKey)
	skid := sha1.Sum(pubASN1)
	client.SubjectKeyId = skid[:]

	certDER, err := x509.CreateCertificate(rand.Reader, client, caCert, &key.PublicKey, caKey)
	if err != nil {
		return err
	}

	savePEM(certPath, "CERTIFICATE", certDER)
	savePEM(keyPath, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(key))
	return nil
}

// -------------------------------------------------------------------

func savePEM(path, typ string, bytes []byte) {
	f, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	_ = pem.Encode(f, &pem.Block{Type: typ, Bytes: bytes})
}

func pemToDer(pemPath, derPath string) {
	data, err := os.ReadFile(pemPath)
	if err != nil {
		panic(err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		panic("❌ Kunne ikke lese PEM-fil")
	}
	_ = os.WriteFile(derPath, block.Bytes, 0644)
}
