/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package certs

import (
	"crypto"
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	certutil "k8s.io/client-go/util/cert"
	"k8s.io/client-go/util/keyutil"
	"k8s.io/klog"
	kubeadmapi "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm"
	pkiutil "k8s.io/kubernetes/cmd/kubeadm/app/util/pkiutil"

	kubeadmconstants "k8s.io/kubernetes/cmd/kubeadm/app/constants"
)

// CreatePKIAssets will create and write to disk all PKI assets necessary to establish the control plane.
// If the PKI assets already exists in the target folder, they are used only if evaluated equal; otherwise an error is returned.
func CreatePKIAssets(cfg *kubeadmapi.InitConfiguration) error {
	klog.V(1).Infoln("creating PKI assets")

	// This structure cannot handle multilevel CA hierarchies.
	// This isn't a problem right now, but may become one in the future.

	var certList Certificates

	if cfg.Etcd.Local == nil {
		certList = GetCertsWithoutEtcd()
	} else {
		certList = GetDefaultCertList()
	}

	certTree, err := certList.AsMap().CertTree()
	if err != nil {
		return err
	}

	if err := certTree.CreateTree(cfg); err != nil {
		return errors.Wrap(err, "error creating PKI assets")
	}

	fmt.Printf("[certs] Valid certificates and keys now exist in %q\n", cfg.CertificatesDir)

	// Service accounts are not x509 certs, so handled separately
	return CreateServiceAccountKeyAndPublicKeyFiles(cfg.CertificatesDir)
}

// CreateServiceAccountKeyAndPublicKeyFiles create a new public/private key files for signing service account users.
// If the sa public/private key files already exists in the target folder, they are used only if evaluated equals; otherwise an error is returned.
func CreateServiceAccountKeyAndPublicKeyFiles(certsDir string) error {
	klog.V(1).Infoln("creating a new public/private key files for signing service account users")
	_, err := keyutil.PrivateKeyFromFile(filepath.Join(certsDir, kubeadmconstants.ServiceAccountPrivateKeyName))
	if err == nil {
		// kubeadm doesn't validate the existing certificate key more than this;
		// Basically, if we find a key file with the same path kubeadm thinks those files
		// are equal and doesn't bother writing a new file
		fmt.Printf("[certs] Using the existing %q key\n", kubeadmconstants.ServiceAccountKeyBaseName)
		return nil
	} else if !os.IsNotExist(err) {
		return errors.Wrapf(err, "file %s existed but it could not be loaded properly", kubeadmconstants.ServiceAccountPrivateKeyName)
	}

	// The key does NOT exist, let's generate it now
	key, err := pkiutil.NewPrivateKey()
	if err != nil {
		return err
	}

	// Write .key and .pub files to disk
	fmt.Printf("[certs] Generating %q key and public key\n", kubeadmconstants.ServiceAccountKeyBaseName)

	if err := pkiutil.WriteKey(certsDir, kubeadmconstants.ServiceAccountKeyBaseName, key); err != nil {
		return err
	}

	return pkiutil.WritePublicKey(certsDir, kubeadmconstants.ServiceAccountKeyBaseName, key.Public())
}

// CreateCACertAndKeyFiles generates and writes out a given certificate authority.
// The certSpec should be one of the variables from this package.
func CreateCACertAndKeyFiles(certSpec *KubeadmCert, cfg *kubeadmapi.InitConfiguration) error {
	if certSpec.CAName != "" {
		return errors.Errorf("this function should only be used for CAs, but cert %s has CA %s", certSpec.Name, certSpec.CAName)
	}
	klog.V(1).Infof("creating a new certificate authority for %s", certSpec.Name)

	certConfig, err := certSpec.GetConfig(cfg)
	if err != nil {
		return err
	}

	caCert, caKey, err := pkiutil.NewCertificateAuthority(certConfig)
	if err != nil {
		return err
	}

	return writeCertificateAuthorityFilesIfNotExist(
		cfg.CertificatesDir,
		certSpec.BaseName,
		caCert,
		caKey,
	)
}

// NewCSR will generate a new CSR and accompanying key
func NewCSR(certSpec *KubeadmCert, cfg *kubeadmapi.InitConfiguration) (*x509.CertificateRequest, crypto.Signer, error) {
	certConfig, err := certSpec.GetConfig(cfg)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to retrieve cert configuration")
	}

	return pkiutil.NewCSRAndKey(certConfig)
}

// CreateCSR creates a certificate signing request
func CreateCSR(certSpec *KubeadmCert, cfg *kubeadmapi.InitConfiguration, path string) error {
	csr, key, err := NewCSR(certSpec, cfg)
	if err != nil {
		return err
	}
	return writeCSRFilesIfNotExist(path, certSpec.BaseName, csr, key)
}

// CreateCertAndKeyFilesWithCA loads the given certificate authority from disk, then generates and writes out the given certificate and key.
// The certSpec and caCertSpec should both be one of the variables from this package.
func CreateCertAndKeyFilesWithCA(certSpec *KubeadmCert, caCertSpec *KubeadmCert, cfg *kubeadmapi.InitConfiguration) error {
	if certSpec.CAName != caCertSpec.Name {
		return errors.Errorf("expected CAname for %s to be %q, but was %s", certSpec.Name, certSpec.CAName, caCertSpec.Name)
	}

	caCert, caKey, err := LoadCertificateAuthority(cfg.CertificatesDir, caCertSpec.BaseName)
	if err != nil {
		return errors.Wrapf(err, "couldn't load CA certificate %s", caCertSpec.Name)
	}

	return certSpec.CreateFromCA(cfg, caCert, caKey)
}

// LoadCertificateAuthority tries to load a CA in the given directory with the given name.
func LoadCertificateAuthority(pkiDir string, baseName string) (*x509.Certificate, crypto.Signer, error) {
	// Checks if certificate authority exists in the PKI directory
	if !pkiutil.CertOrKeyExist(pkiDir, baseName) {
		return nil, nil, errors.Errorf("couldn't load %s certificate authority from %s", baseName, pkiDir)
	}

	// Try to load certificate authority .crt and .key from the PKI directory
	caCert, caKey, err := pkiutil.TryLoadCertAndKeyFromDisk(pkiDir, baseName)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failure loading %s certificate authority", baseName)
	}

	// Make sure the loaded CA cert actually is a CA
	if !caCert.IsCA {
		return nil, nil, errors.Errorf("%s certificate is not a certificate authority", baseName)
	}

	return caCert, caKey, nil
}

// writeCertificateAuthorityFilesIfNotExist write a new certificate Authority to the given path.
// If there already is a certificate file at the given path; kubeadm tries to load it and check if the values in the
// existing and the expected certificate equals. If they do; kubeadm will just skip writing the file as it's up-to-date,
// otherwise this function returns an error.
func writeCertificateAuthorityFilesIfNotExist(pkiDir string, baseName string, caCert *x509.Certificate, caKey crypto.Signer) error {

	// If cert or key exists, we should try to load them
	if pkiutil.CertOrKeyExist(pkiDir, baseName) {

		// Try to load .crt and .key from the PKI directory
		caCert, _, err := pkiutil.TryLoadCertAndKeyFromDisk(pkiDir, baseName)
		if err != nil {
			return errors.Wrapf(err, "failure loading %s certificate", baseName)
		}

		// Check if the existing cert is a CA
		if !caCert.IsCA {
			return errors.Errorf("certificate %s is not a CA", baseName)
		}

		// kubeadm doesn't validate the existing certificate Authority more than this;
		// Basically, if we find a certificate file with the same path; and it is a CA
		// kubeadm thinks those files are equal and doesn't bother writing a new file
		fmt.Printf("[certs] Using the existing %q certificate and key\n", baseName)
	} else {
		// Write .crt and .key files to disk
		fmt.Printf("[certs] Generating %q certificate and key\n", baseName)

		if err := pkiutil.WriteCertAndKey(pkiDir, baseName, caCert, caKey); err != nil {
			return errors.Wrapf(err, "failure while saving %s certificate and key", baseName)
		}
	}
	return nil
}

// writeCertificateFilesIfNotExist write a new certificate to the given path.
// If there already is a certificate file at the given path; kubeadm tries to load it and check if the values in the
// existing and the expected certificate equals. If they do; kubeadm will just skip writing the file as it's up-to-date,
// otherwise this function returns an error.
func writeCertificateFilesIfNotExist(pkiDir string, baseName string, signingCert *x509.Certificate, cert *x509.Certificate, key crypto.Signer, cfg *certutil.Config) error {

	// Checks if the signed certificate exists in the PKI directory
	if pkiutil.CertOrKeyExist(pkiDir, baseName) {
		// Try to load signed certificate .crt and .key from the PKI directory
		signedCert, _, err := pkiutil.TryLoadCertAndKeyFromDisk(pkiDir, baseName)
		if err != nil {
			return errors.Wrapf(err, "failure loading %s certificate", baseName)
		}

		// Check if the existing cert is signed by the given CA
		if err := signedCert.CheckSignatureFrom(signingCert); err != nil {
			return errors.Errorf("certificate %s is not signed by corresponding CA", baseName)
		}

		// Check if the certificate has the correct attributes
		if err := validateCertificateWithConfig(signedCert, baseName, cfg); err != nil {
			return err
		}

		fmt.Printf("[certs] Using the existing %q certificate and key\n", baseName)
	} else {
		// Write .crt and .key files to disk
		fmt.Printf("[certs] Generating %q certificate and key\n", baseName)

		if err := pkiutil.WriteCertAndKey(pkiDir, baseName, cert, key); err != nil {
			return errors.Wrapf(err, "failure while saving %s certificate and key", baseName)
		}
		if pkiutil.HasServerAuth(cert) {
			fmt.Printf("[certs] %s serving cert is signed for DNS names %v and IPs %v\n", baseName, cert.DNSNames, cert.IPAddresses)
		}
	}

	return nil
}

// writeCSRFilesIfNotExist writes a new CSR to the given path.
// If there already is a CSR file at the given path; kubeadm tries to load it and check if it's a valid certificate.
// otherwise this function returns an error.
func writeCSRFilesIfNotExist(csrDir string, baseName string, csr *x509.CertificateRequest, key crypto.Signer) error {
	if pkiutil.CSROrKeyExist(csrDir, baseName) {
		_, _, err := pkiutil.TryLoadCSRAndKeyFromDisk(csrDir, baseName)
		if err != nil {
			return errors.Wrapf(err, "%s CSR existed but it could not be loaded properly", baseName)
		}

		fmt.Printf("[certs] Using the existing %q CSR\n", baseName)
	} else {
		// Write .key and .csr files to disk
		fmt.Printf("[certs] Generating %q key and CSR\n", baseName)

		if err := pkiutil.WriteKey(csrDir, baseName, key); err != nil {
			return errors.Wrapf(err, "failure while saving %s key", baseName)
		}

		if err := pkiutil.WriteCSR(csrDir, baseName, csr); err != nil {
			return errors.Wrapf(err, "failure while saving %s CSR", baseName)
		}
	}

	return nil
}

type certKeyLocation struct {
	pkiDir     string
	caBaseName string
	baseName   string
	uxName     string
}

// SharedCertificateExists verifies if the shared certificates - the certificates that must be
// equal across control-plane nodes: ca.key, ca.crt, sa.key, sa.pub + etcd/ca.key, etcd/ca.crt if local/stacked etcd
func SharedCertificateExists(cfg *kubeadmapi.ClusterConfiguration) (bool, error) {

	if err := validateCACertAndKey(certKeyLocation{cfg.CertificatesDir, kubeadmconstants.CACertAndKeyBaseName, "", "CA"}); err != nil {
		return false, err
	}

	if err := validatePrivatePublicKey(certKeyLocation{cfg.CertificatesDir, "", kubeadmconstants.ServiceAccountKeyBaseName, "service account"}); err != nil {
		return false, err
	}

	if err := validateCACertAndKey(certKeyLocation{cfg.CertificatesDir, kubeadmconstants.FrontProxyCACertAndKeyBaseName, "", "front-proxy CA"}); err != nil {
		return false, err
	}

	// in case of local/stacked etcd
	if cfg.Etcd.External == nil {
		if err := validateCACertAndKey(certKeyLocation{cfg.CertificatesDir, kubeadmconstants.EtcdCACertAndKeyBaseName, "", "etcd CA"}); err != nil {
			return false, err
		}
	}

	return true, nil
}

// UsingExternalCA determines whether the user is relying on an external CA.  We currently implicitly determine this is the case
// when the CA Cert is present but the CA Key is not.
// This allows us to, e.g., skip generating certs or not start the csr signing controller.
// In case we are using an external front-proxy CA, the function validates the certificates signed by front-proxy CA that should be provided by the user.
func UsingExternalCA(cfg *kubeadmapi.ClusterConfiguration) (bool, error) {

	if err := validateCACert(certKeyLocation{cfg.CertificatesDir, kubeadmconstants.CACertAndKeyBaseName, "", "CA"}); err != nil {
		return false, err
	}

	caKeyPath := filepath.Join(cfg.CertificatesDir, kubeadmconstants.CAKeyName)
	if _, err := os.Stat(caKeyPath); !os.IsNotExist(err) {
		return false, nil
	}

	if err := validateSignedCert(certKeyLocation{cfg.CertificatesDir, kubeadmconstants.CACertAndKeyBaseName, kubeadmconstants.APIServerCertAndKeyBaseName, "API server"}); err != nil {
		return true, err
	}

	if err := validateSignedCert(certKeyLocation{cfg.CertificatesDir, kubeadmconstants.CACertAndKeyBaseName, kubeadmconstants.APIServerKubeletClientCertAndKeyBaseName, "API server kubelet client"}); err != nil {
		return true, err
	}

	return true, nil
}

// UsingExternalFrontProxyCA determines whether the user is relying on an external front-proxy CA.  We currently implicitly determine this is the case
// when the front proxy CA Cert is present but the front proxy CA Key is not.
// In case we are using an external front-proxy CA, the function validates the certificates signed by front-proxy CA that should be provided by the user.
func UsingExternalFrontProxyCA(cfg *kubeadmapi.ClusterConfiguration) (bool, error) {

	if err := validateCACert(certKeyLocation{cfg.CertificatesDir, kubeadmconstants.FrontProxyCACertAndKeyBaseName, "", "front-proxy CA"}); err != nil {
		return false, err
	}

	frontProxyCAKeyPath := filepath.Join(cfg.CertificatesDir, kubeadmconstants.FrontProxyCAKeyName)
	if _, err := os.Stat(frontProxyCAKeyPath); !os.IsNotExist(err) {
		return false, nil
	}

	if err := validateSignedCert(certKeyLocation{cfg.CertificatesDir, kubeadmconstants.FrontProxyCACertAndKeyBaseName, kubeadmconstants.FrontProxyClientCertAndKeyBaseName, "front-proxy client"}); err != nil {
		return true, err
	}

	return true, nil
}

// validateCACert tries to load a x509 certificate from pkiDir and validates that it is a CA
func validateCACert(l certKeyLocation) error {
	// Check CA Cert
	caCert, err := pkiutil.TryLoadCertFromDisk(l.pkiDir, l.caBaseName)
	if err != nil {
		return errors.Wrapf(err, "failure loading certificate for %s", l.uxName)
	}

	// Check if cert is a CA
	if !caCert.IsCA {
		return errors.Errorf("certificate %s is not a CA", l.uxName)
	}
	return nil
}

// validateCACertAndKey tries to load a x509 certificate and private key from pkiDir,
// and validates that the cert is a CA
func validateCACertAndKey(l certKeyLocation) error {
	if err := validateCACert(l); err != nil {
		return err
	}

	_, err := pkiutil.TryLoadKeyFromDisk(l.pkiDir, l.caBaseName)
	if err != nil {
		return errors.Wrapf(err, "failure loading key for %s", l.uxName)
	}
	return nil
}

// validateSignedCert tries to load a x509 certificate and private key from pkiDir and validates
// that the cert is signed by a given CA
func validateSignedCert(l certKeyLocation) error {
	// Try to load CA
	caCert, err := pkiutil.TryLoadCertFromDisk(l.pkiDir, l.caBaseName)
	if err != nil {
		return errors.Wrapf(err, "failure loading certificate authority for %s", l.uxName)
	}

	return validateSignedCertWithCA(l, caCert)
}

// validateSignedCertWithCA tries to load a certificate and validate it with the given caCert
func validateSignedCertWithCA(l certKeyLocation, caCert *x509.Certificate) error {
	// Try to load key and signed certificate
	signedCert, _, err := pkiutil.TryLoadCertAndKeyFromDisk(l.pkiDir, l.baseName)
	if err != nil {
		return errors.Wrapf(err, "failure loading certificate for %s", l.uxName)
	}

	// Check if the cert is signed by the CA
	if err := signedCert.CheckSignatureFrom(caCert); err != nil {
		return errors.Wrapf(err, "certificate %s is not signed by corresponding CA", l.uxName)
	}
	return nil
}

// validatePrivatePublicKey tries to load a private key from pkiDir
func validatePrivatePublicKey(l certKeyLocation) error {
	// Try to load key
	_, _, err := pkiutil.TryLoadPrivatePublicKeyFromDisk(l.pkiDir, l.baseName)
	if err != nil {
		return errors.Wrapf(err, "failure loading key for %s", l.uxName)
	}
	return nil
}

// validateCertificateWithConfig makes sure that a given certificate is valid at
// least for the SANs defined in the configuration.
func validateCertificateWithConfig(cert *x509.Certificate, baseName string, cfg *certutil.Config) error {
	for _, dnsName := range cfg.AltNames.DNSNames {
		if err := cert.VerifyHostname(dnsName); err != nil {
			return errors.Wrapf(err, "certificate %s is invalid", baseName)
		}
	}
	for _, ipAddress := range cfg.AltNames.IPs {
		if err := cert.VerifyHostname(ipAddress.String()); err != nil {
			return errors.Wrapf(err, "certificate %s is invalid", baseName)
		}
	}
	return nil
}
