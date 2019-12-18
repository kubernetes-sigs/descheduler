/*
Copyright 2017 The Kubernetes Authors.

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

package encryptionconfig

import (
	"bytes"
	"encoding/base64"
	"io"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/diff"
	apiserverconfig "k8s.io/apiserver/pkg/apis/config"
	"k8s.io/apiserver/pkg/storage/value"
	"k8s.io/apiserver/pkg/storage/value/encrypt/envelope"
)

const (
	sampleText        = "abcdefghijklmnopqrstuvwxyz"
	sampleContextText = "0123456789"
)

func mustReadConfig(t *testing.T, path string) []byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("error opening encryption configuration file %q: %v", path, err)
	}
	defer f.Close()

	configFileContents, err := ioutil.ReadAll(f)
	if err != nil {
		t.Fatalf("could not read contents of encryption config: %v", err)
	}

	return configFileContents
}

func mustConfigReader(t *testing.T, path string) io.Reader {
	return bytes.NewReader(mustReadConfig(t, path))
}

// testEnvelopeService is a mock envelope service which can be used to simulate remote Envelope services
// for testing of the envelope transformer with other transformers.
type testEnvelopeService struct {
}

func (t *testEnvelopeService) Decrypt(data []byte) ([]byte, error) {
	return base64.StdEncoding.DecodeString(string(data))
}

func (t *testEnvelopeService) Encrypt(data []byte) ([]byte, error) {
	return []byte(base64.StdEncoding.EncodeToString(data)), nil
}

// The factory method to create mock envelope service.
func newMockEnvelopeService(endpoint string, timeout time.Duration) (envelope.Service, error) {
	return &testEnvelopeService{}, nil
}

func TestLegacyConfig(t *testing.T) {
	legacyV1Config := "testdata/valid-configs/legacy.yaml"
	legacyConfigObject, err := loadConfig(mustReadConfig(t, legacyV1Config))
	if err != nil {
		t.Fatalf("error while parsing configuration file: %s.\nThe file was:\n%s", err, legacyV1Config)
	}

	expected := &apiserverconfig.EncryptionConfiguration{
		Resources: []apiserverconfig.ResourceConfiguration{
			{
				Resources: []string{"secrets", "namespaces"},
				Providers: []apiserverconfig.ProviderConfiguration{
					{Identity: &apiserverconfig.IdentityConfiguration{}},
					{AESGCM: &apiserverconfig.AESConfiguration{
						Keys: []apiserverconfig.Key{
							{Name: "key1", Secret: "c2VjcmV0IGlzIHNlY3VyZQ=="},
							{Name: "key2", Secret: "dGhpcyBpcyBwYXNzd29yZA=="},
						},
					}},
					{KMS: &apiserverconfig.KMSConfiguration{
						Name:      "testprovider",
						Endpoint:  "unix:///tmp/testprovider.sock",
						CacheSize: 10,
					}},
					{AESCBC: &apiserverconfig.AESConfiguration{
						Keys: []apiserverconfig.Key{
							{Name: "key1", Secret: "c2VjcmV0IGlzIHNlY3VyZQ=="},
							{Name: "key2", Secret: "dGhpcyBpcyBwYXNzd29yZA=="},
						},
					}},
					{Secretbox: &apiserverconfig.SecretboxConfiguration{
						Keys: []apiserverconfig.Key{
							{Name: "key1", Secret: "YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXoxMjM0NTY="},
						},
					}},
				},
			},
		},
	}
	if !reflect.DeepEqual(legacyConfigObject, expected) {
		t.Fatal(diff.ObjectReflectDiff(expected, legacyConfigObject))
	}
}

func TestEncryptionProviderConfigCorrect(t *testing.T) {
	// Set factory for mock envelope service
	factory := envelopeServiceFactory
	envelopeServiceFactory = newMockEnvelopeService
	defer func() {
		envelopeServiceFactory = factory
	}()

	// Creates compound/prefix transformers with different ordering of available transformers.
	// Transforms data using one of them, and tries to untransform using the others.
	// Repeats this for all possible combinations.
	correctConfigWithIdentityFirst := "testdata/valid-configs/identity-first.yaml"
	identityFirstTransformerOverrides, err := ParseEncryptionConfiguration(mustConfigReader(t, correctConfigWithIdentityFirst))
	if err != nil {
		t.Fatalf("error while parsing configuration file: %s.\nThe file was:\n%s", err, correctConfigWithIdentityFirst)
	}

	correctConfigWithAesGcmFirst := "testdata/valid-configs/aes-gcm-first.yaml"
	aesGcmFirstTransformerOverrides, err := ParseEncryptionConfiguration(mustConfigReader(t, correctConfigWithAesGcmFirst))
	if err != nil {
		t.Fatalf("error while parsing configuration file: %s.\nThe file was:\n%s", err, correctConfigWithAesGcmFirst)
	}

	correctConfigWithAesCbcFirst := "testdata/valid-configs/aes-cbc-first.yaml"
	aesCbcFirstTransformerOverrides, err := ParseEncryptionConfiguration(mustConfigReader(t, correctConfigWithAesCbcFirst))
	if err != nil {
		t.Fatalf("error while parsing configuration file: %s.\nThe file was:\n%s", err, correctConfigWithAesCbcFirst)
	}

	correctConfigWithSecretboxFirst := "testdata/valid-configs/secret-box-first.yaml"
	secretboxFirstTransformerOverrides, err := ParseEncryptionConfiguration(mustConfigReader(t, correctConfigWithSecretboxFirst))
	if err != nil {
		t.Fatalf("error while parsing configuration file: %s.\nThe file was:\n%s", err, correctConfigWithSecretboxFirst)
	}

	correctConfigWithKMSFirst := "testdata/valid-configs/kms-first.yaml"
	kmsFirstTransformerOverrides, err := ParseEncryptionConfiguration(mustConfigReader(t, correctConfigWithKMSFirst))
	if err != nil {
		t.Fatalf("error while parsing configuration file: %s.\nThe file was:\n%s", err, correctConfigWithKMSFirst)
	}

	// Pick the transformer for any of the returned resources.
	identityFirstTransformer := identityFirstTransformerOverrides[schema.ParseGroupResource("secrets")]
	aesGcmFirstTransformer := aesGcmFirstTransformerOverrides[schema.ParseGroupResource("secrets")]
	aesCbcFirstTransformer := aesCbcFirstTransformerOverrides[schema.ParseGroupResource("secrets")]
	secretboxFirstTransformer := secretboxFirstTransformerOverrides[schema.ParseGroupResource("secrets")]
	kmsFirstTransformer := kmsFirstTransformerOverrides[schema.ParseGroupResource("secrets")]

	context := value.DefaultContext([]byte(sampleContextText))
	originalText := []byte(sampleText)

	transformers := []struct {
		Transformer value.Transformer
		Name        string
	}{
		{aesGcmFirstTransformer, "aesGcmFirst"},
		{aesCbcFirstTransformer, "aesCbcFirst"},
		{secretboxFirstTransformer, "secretboxFirst"},
		{identityFirstTransformer, "identityFirst"},
		{kmsFirstTransformer, "kmsFirst"},
	}

	for _, testCase := range transformers {
		transformedData, err := testCase.Transformer.TransformToStorage(originalText, context)
		if err != nil {
			t.Fatalf("%s: error while transforming data to storage: %s", testCase.Name, err)
		}

		for _, transformer := range transformers {
			untransformedData, stale, err := transformer.Transformer.TransformFromStorage(transformedData, context)
			if err != nil {
				t.Fatalf("%s: error while reading using %s transformer: %s", testCase.Name, transformer.Name, err)
			}
			if stale != (transformer.Name != testCase.Name) {
				t.Fatalf("%s: wrong stale information on reading using %s transformer, should be %v", testCase.Name, transformer.Name, testCase.Name == transformer.Name)
			}
			if !bytes.Equal(untransformedData, originalText) {
				t.Fatalf("%s: %s transformer transformed data incorrectly. Expected: %v, got %v", testCase.Name, transformer.Name, originalText, untransformedData)
			}
		}
	}
}

// Throw error if key has no secret
func TestEncryptionProviderConfigNoSecretForKey(t *testing.T) {
	incorrectConfigNoSecretForKey := "testdata/invalid-configs/aes/no-key.yaml"
	if _, err := ParseEncryptionConfiguration(mustConfigReader(t, incorrectConfigNoSecretForKey)); err == nil {
		t.Fatalf("invalid configuration file (one key has no secret) got parsed:\n%s", incorrectConfigNoSecretForKey)
	}
}

// Throw error if invalid key for AES
func TestEncryptionProviderConfigInvalidKey(t *testing.T) {
	incorrectConfigInvalidKey := "testdata/invalid-configs/aes/invalid-key.yaml"
	if _, err := ParseEncryptionConfiguration(mustConfigReader(t, incorrectConfigInvalidKey)); err == nil {
		t.Fatalf("invalid configuration file (bad AES key) got parsed:\n%s", incorrectConfigInvalidKey)
	}
}

// Throw error if kms has no endpoint
func TestEncryptionProviderConfigNoEndpointForKMS(t *testing.T) {
	incorrectConfigNoEndpointForKMS := "testdata/invalid-configs/kms/no-endpoint.yaml"
	if _, err := ParseEncryptionConfiguration(mustConfigReader(t, incorrectConfigNoEndpointForKMS)); err == nil {
		t.Fatalf("invalid configuration file (kms has no endpoint) got parsed:\n%s", incorrectConfigNoEndpointForKMS)
	}
}

func TestKMSConfigTimeout(t *testing.T) {
	testCases := []struct {
		desc    string
		config  string
		want    time.Duration
		wantErr string
	}{
		{
			desc:   "duration explicitly provided",
			config: "testdata/valid-configs/kms/valid-timeout.yaml",
			want:   15 * time.Second,
		},
		{
			desc:    "duration explicitly provided as 0 which is an invalid value, error should be returned",
			config:  "testdata/invalid-configs/kms/zero-timeout.yaml",
			wantErr: "timeout should be a positive value",
		},
		{
			desc:   "duration is not provided, default will be supplied",
			config: "testdata/valid-configs/kms/default-timeout.yaml",
			want:   kmsPluginConnectionTimeout,
		},
		{
			desc:    "duration is invalid (negative), error should be returned",
			config:  "testdata/invalid-configs/kms/negative-timeout.yaml",
			wantErr: "timeout should be a positive value",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.desc, func(t *testing.T) {
			// mocking envelopeServiceFactory to sense the value of the supplied timeout.
			envelopeServiceFactory = func(endpoint string, callTimeout time.Duration) (envelope.Service, error) {
				if callTimeout != tt.want {
					t.Fatalf("got timeout: %v, want %v", callTimeout, tt.want)
				}

				return newMockEnvelopeService(endpoint, callTimeout)
			}

			// mocked envelopeServiceFactory is called during ParseEncryptionConfiguration.
			if _, err := ParseEncryptionConfiguration(mustConfigReader(t, tt.config)); err != nil && !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("unable to parse yaml\n%s\nerror: %v", tt.config, err)
			}
		})
	}
}

func TestKMSPluginHealthz(t *testing.T) {
	service, err := envelope.NewGRPCService("unix:///tmp/testprovider.sock", kmsPluginConnectionTimeout)
	if err != nil {
		t.Fatalf("Could not initialize envelopeService, error: %v", err)
	}

	testCases := []struct {
		desc    string
		config  string
		want    []*kmsPluginProbe
		wantErr bool
	}{
		{
			desc:   "Install Healthz",
			config: "testdata/valid-configs/kms/default-timeout.yaml",
			want: []*kmsPluginProbe{
				{
					name:    "foo",
					Service: service,
				},
			},
		},
		{
			desc:   "Install multiple healthz",
			config: "testdata/valid-configs/kms/multiple-providers.yaml",
			want: []*kmsPluginProbe{
				{
					name:    "foo",
					Service: service,
				},
				{
					name:    "bar",
					Service: service,
				},
			},
		},
		{
			desc:   "No KMS Providers",
			config: "testdata/valid-configs/aes/aes-gcm.yaml",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.desc, func(t *testing.T) {
			got, err := getKMSPluginProbes(mustConfigReader(t, tt.config))
			if err != nil && !tt.wantErr {
				t.Fatalf("got %v, want nil for error", err)
			}

			if d := cmp.Diff(tt.want, got, cmp.Comparer(serviceComparer)); d != "" {
				t.Fatalf("HealthzConfig mismatch (-want +got):\n%s", d)
			}
		})
	}
}

// As long as got and want contain envelope.Service we will return true.
// If got has an envelope.Service and want does note (or vice versa) this will return false.
func serviceComparer(_, _ envelope.Service) bool {
	return true
}

func TestCBCKeyRotationWithOverlappingProviders(t *testing.T) {
	testCBCKeyRotationWithProviders(
		t,
		"testdata/valid-configs/aes/aes-cbc-multiple-providers.json",
		"k8s:enc:aescbc:v1:1:",
		"testdata/valid-configs/aes/aes-cbc-multiple-providers-reversed.json",
		"k8s:enc:aescbc:v1:2:",
	)
}

func TestCBCKeyRotationWithoutOverlappingProviders(t *testing.T) {
	testCBCKeyRotationWithProviders(
		t,
		"testdata/valid-configs/aes/aes-cbc-multiple-keys.json",
		"k8s:enc:aescbc:v1:A:",
		"testdata/valid-configs/aes/aes-cbc-multiple-keys-reversed.json",
		"k8s:enc:aescbc:v1:B:",
	)
}

func testCBCKeyRotationWithProviders(t *testing.T, firstEncryptionConfig, firstPrefix, secondEncryptionConfig, secondPrefix string) {
	p := getTransformerFromEncryptionConfig(t, firstEncryptionConfig)

	context := value.DefaultContext([]byte("authenticated_data"))

	out, err := p.TransformToStorage([]byte("firstvalue"), context)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(out, []byte(firstPrefix)) {
		t.Fatalf("unexpected prefix: %q", out)
	}
	from, stale, err := p.TransformFromStorage(out, context)
	if err != nil {
		t.Fatal(err)
	}
	if stale || !bytes.Equal([]byte("firstvalue"), from) {
		t.Fatalf("unexpected data: %t %q", stale, from)
	}

	// verify changing the context fails storage
	_, _, err = p.TransformFromStorage(out, value.DefaultContext([]byte("incorrect_context")))
	if err != nil {
		t.Fatalf("CBC mode does not support authentication: %v", err)
	}

	// reverse the order, use the second key
	p = getTransformerFromEncryptionConfig(t, secondEncryptionConfig)
	from, stale, err = p.TransformFromStorage(out, context)
	if err != nil {
		t.Fatal(err)
	}
	if !stale || !bytes.Equal([]byte("firstvalue"), from) {
		t.Fatalf("unexpected data: %t %q", stale, from)
	}

	out, err = p.TransformToStorage([]byte("firstvalue"), context)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(out, []byte(secondPrefix)) {
		t.Fatalf("unexpected prefix: %q", out)
	}
	from, stale, err = p.TransformFromStorage(out, context)
	if err != nil {
		t.Fatal(err)
	}
	if stale || !bytes.Equal([]byte("firstvalue"), from) {
		t.Fatalf("unexpected data: %t %q", stale, from)
	}
}

func getTransformerFromEncryptionConfig(t *testing.T, encryptionConfigPath string) value.Transformer {
	t.Helper()
	transformers, err := ParseEncryptionConfiguration(mustConfigReader(t, encryptionConfigPath))
	if err != nil {
		t.Fatal(err)
	}
	if len(transformers) != 1 {
		t.Fatalf("input config does not have exactly one resource: %s", encryptionConfigPath)
	}
	for _, transformer := range transformers {
		return transformer
	}
	panic("unreachable")
}
