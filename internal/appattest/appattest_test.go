package appattest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
)

// Vectors produced by a real device, from github.com/bas-d/appattest
// (BSD-3-Clause, Copyright (c) 2019, Duo Labs).
type deviceVectors struct {
	AppID                 string `json:"app_id"`
	KeyID                 string `json:"key_id"`
	Attestation           string `json:"attestation"`
	AttestationClientData string `json:"attestation_client_data"`
	AttestedAt            string `json:"attested_at"`
	PublicKey             string `json:"public_key"`
	Assertion             string `json:"assertion"`
	AssertionClientData   string `json:"assertion_client_data"`
}

func loadVectors(t *testing.T) (deviceVectors, []byte, []byte, []byte, []byte, time.Time) {
	t.Helper()
	raw, err := os.ReadFile("testdata/device_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var v deviceVectors
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	keyID, err := base64.StdEncoding.DecodeString(v.KeyID)
	if err != nil {
		t.Fatal(err)
	}
	att, err := base64.RawURLEncoding.DecodeString(v.Attestation)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := hex.DecodeString(v.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	as, err := base64.RawURLEncoding.DecodeString(v.Assertion)
	if err != nil {
		t.Fatal(err)
	}
	at, err := time.Parse(time.RFC3339, v.AttestedAt)
	if err != nil {
		t.Fatal(err)
	}
	return v, keyID, att, pub, as, at
}

func TestVerifyAttestation(t *testing.T) {
	v, keyID, att, pub, _, at := loadVectors(t)
	cdh := sha256.Sum256([]byte(v.AttestationClientData))
	dev := Policy{AppIDs: []string{"OTHERTEAM.com.example", v.AppID}, AllowDevelopment: true}

	key, err := VerifyAttestation(att, keyID, cdh[:], dev, at)
	if err != nil {
		t.Fatalf("device attestation refused: %v", err)
	}
	if hex.EncodeToString(key.PublicKey) != hex.EncodeToString(pub) {
		t.Fatalf("public key = %x", key.PublicKey)
	}
	if key.AppID != v.AppID || key.Environment != Development {
		t.Fatalf("app id %q, environment %q", key.AppID, key.Environment)
	}

	otherHash := sha256.Sum256([]byte("another-challenge"))
	otherKey := append([]byte{}, keyID...)
	otherKey[0] ^= 1
	cases := map[string]struct {
		keyID, cdh []byte
		policy     Policy
		now        time.Time
	}{
		"development refused":   {keyID, cdh[:], Policy{AppIDs: []string{v.AppID}}, at},
		"another app":           {keyID, cdh[:], Policy{AppIDs: []string{"OTHERTEAM.com.example"}, AllowDevelopment: true}, at},
		"another challenge":     {keyID, otherHash[:], dev, at},
		"another key id":        {otherKey, cdh[:], dev, at},
		"expired credential":    {keyID, cdh[:], dev, at.AddDate(0, 1, 0)},
		"no accepted app ids":   {keyID, cdh[:], Policy{AllowDevelopment: true}, at},
		"truncated attestation": {keyID, cdh[:], dev, at},
	}
	for name, tc := range cases {
		object := att
		if name == "truncated attestation" {
			object = att[:len(att)/2]
		}
		if _, err := VerifyAttestation(object, tc.keyID, tc.cdh, tc.policy, tc.now); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}

func TestVerifyAssertion(t *testing.T) {
	v, _, _, pub, as, _ := loadVectors(t)
	data := []byte(v.AssertionClientData)

	counter, err := VerifyAssertion(as, data, pub, v.AppID, 0, true)
	if err != nil {
		t.Fatalf("device assertion refused: %v", err)
	}
	if counter == 0 {
		t.Fatal("counter did not advance")
	}

	if _, err := VerifyAssertion(as, data, pub, v.AppID, counter, true); !errors.Is(err, ErrInvalid) {
		t.Errorf("replayed counter: err = %v", err)
	}
	if _, err := VerifyAssertion(as, []byte(`{"challenge":"other"}`), pub, v.AppID, 0, true); !errors.Is(err, ErrInvalid) {
		t.Errorf("other client data: err = %v", err)
	}
	if _, err := VerifyAssertion(as, data, pub, "OTHERTEAM.com.example", 0, true); !errors.Is(err, ErrInvalid) {
		t.Errorf("other app id: err = %v", err)
	}
	tampered := append([]byte{}, as...)
	tampered[len(tampered)-1] ^= 1
	if _, err := VerifyAssertion(tampered, data, pub, v.AppID, 0, true); !errors.Is(err, ErrInvalid) {
		t.Errorf("tampered assertion: err = %v", err)
	}
}

func TestValidationCategory(t *testing.T) {
	ad := &authenticatorData{extensions: map[string]any{}}
	for category, want := range map[uint64]bool{2: true, 3: false, 4: true, 5: false, 10: false, 0: false} {
		ad.extensions[extValidationCategory] = category
		if got := ad.checkCategory(false) == nil; got != want {
			t.Errorf("category %d accepted = %v", category, got)
		}
	}
	ad.extensions[extValidationCategory] = uint64(3)
	if err := ad.checkCategory(true); err != nil {
		t.Errorf("development build refused with development allowed: %v", err)
	}
	delete(ad.extensions, extValidationCategory)
	if err := ad.checkCategory(false); err != nil {
		t.Errorf("no category reported: %v", err)
	}
}

// synthetic builds a production attestation under a throwaway CA, so the production
// path and the refusals the device vectors cannot reach are covered too.
type synthetic struct {
	roots   *x509.CertPool
	chain   [][]byte
	leafKey *ecdsa.PrivateKey
	keyID   []byte
	point   []byte
}

func newSynthetic(t *testing.T, appID string, aaguid []byte, counter uint32, challenge string, tamper func(*attestationObject)) (synthetic, []byte) {
	t.Helper()
	mk := func(tmpl, parent *x509.Certificate, pub, signer any) *x509.Certificate {
		der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, pub, signer)
		if err != nil {
			t.Fatal(err)
		}
		c, _ := x509.ParseCertificate(der)
		return c
	}
	now := time.Now()
	rootKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	rootT := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "root"}, NotBefore: now.Add(-time.Hour),
		NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	root := mk(rootT, rootT, &rootKey.PublicKey, rootKey)
	interKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	interT := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "inter"}, NotBefore: now.Add(-time.Hour),
		NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	inter := mk(interT, root, &interKey.PublicKey, rootKey)

	leafKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	ek, _ := leafKey.PublicKey.ECDH()
	point := ek.Bytes()
	kid := sha256.Sum256(point)

	cose, _ := cbor.Marshal(map[int]any{1: 2, 3: -7, -1: 1, -2: point[1:33], -3: point[33:]})
	rp := sha256.Sum256([]byte(appID))
	ad := append([]byte{}, rp[:]...)
	ad = append(ad, flagAttestedData)
	ad = binary.BigEndian.AppendUint32(ad, counter)
	ad = append(ad, aaguid...)
	ad = binary.BigEndian.AppendUint16(ad, uint16(len(kid)))
	ad = append(ad, kid[:]...)
	ad = append(ad, cose...)

	cdh := sha256.Sum256([]byte(challenge))
	nonce := sha256.Sum256(append(append([]byte{}, ad...), cdh[:]...))
	ext, _ := asn1.Marshal(struct {
		Nonce []byte `asn1:"tag:1,explicit"`
	}{nonce[:]})
	leafT := &x509.Certificate{SerialNumber: big.NewInt(3), Subject: pkix.Name{CommonName: "leaf"}, NotBefore: now.Add(-time.Hour),
		NotAfter: now.Add(time.Hour), ExtraExtensions: []pkix.Extension{{Id: nonceExtensionOID, Value: ext}}}
	leaf := mk(leafT, inter, &leafKey.PublicKey, interKey)

	roots := x509.NewCertPool()
	roots.AddCert(root)
	att := attestationObject{Fmt: "apple-appattest", AuthData: ad}
	att.AttStmt.X5C = [][]byte{leaf.Raw, inter.Raw}
	if tamper != nil {
		tamper(&att)
	}
	obj, err := cbor.Marshal(att)
	if err != nil {
		t.Fatal(err)
	}
	return synthetic{roots: roots, leafKey: leafKey, keyID: kid[:], point: point}, obj
}

func (s synthetic) assert(t *testing.T, appID string, counter uint32, clientData []byte) []byte {
	t.Helper()
	rp := sha256.Sum256([]byte(appID))
	ad := append(append([]byte{}, rp[:]...), 0)
	ad = binary.BigEndian.AppendUint32(ad, counter)
	cdh := sha256.Sum256(clientData)
	nonce := sha256.Sum256(append(append([]byte{}, ad...), cdh[:]...))
	digest := sha256.Sum256(nonce[:])
	sig, err := ecdsa.SignASN1(rand.Reader, s.leafKey, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	obj, _ := cbor.Marshal(assertionObject{Signature: sig, AuthenticatorData: ad})
	return obj
}

func TestSyntheticProduction(t *testing.T) {
	const app = "TEAM123456.sh.opendesk.app"
	const challenge = "c2FtcGxlLWNoYWxsZW5nZQ"
	syn, obj := newSynthetic(t, app, aaguidProduction, 0, challenge, nil)
	cdh := sha256.Sum256([]byte(challenge))
	policy := Policy{AppIDs: []string{app}, Roots: syn.roots}
	key, err := VerifyAttestation(obj, syn.keyID, cdh[:], policy, time.Now())
	if err != nil {
		t.Fatalf("production attestation refused: %v", err)
	}
	if key.Environment != Production || key.AppID != app {
		t.Fatalf("key = %+v", key)
	}
	if _, err := VerifyAttestation(obj, syn.keyID, cdh[:], Policy{AppIDs: []string{app}}, time.Now()); !errors.Is(err, ErrInvalid) {
		t.Errorf("chain outside Apple's root accepted: %v", err)
	}

	refused := map[string]func() ([]byte, synthetic){
		"counter not zero": func() ([]byte, synthetic) {
			s, o := newSynthetic(t, app, aaguidProduction, 1, challenge, nil)
			return o, s
		},
		"unknown aaguid": func() ([]byte, synthetic) {
			s, o := newSynthetic(t, app, []byte("notappattest0000"), 0, challenge, nil)
			return o, s
		},
		"development without the flag": func() ([]byte, synthetic) {
			s, o := newSynthetic(t, app, aaguidDevelopment, 0, challenge, nil)
			return o, s
		},
		"other format": func() ([]byte, synthetic) {
			s, o := newSynthetic(t, app, aaguidProduction, 0, challenge, func(a *attestationObject) { a.Fmt = "packed" })
			return o, s
		},
		"single certificate": func() ([]byte, synthetic) {
			s, o := newSynthetic(t, app, aaguidProduction, 0, challenge, func(a *attestationObject) { a.AttStmt.X5C = a.AttStmt.X5C[:1] })
			return o, s
		},
		"another app's rp id": func() ([]byte, synthetic) {
			s, o := newSynthetic(t, "OTHER.app", aaguidProduction, 0, challenge, nil)
			return o, s
		},
	}
	for name, build := range refused {
		o, s := build()
		if _, err := VerifyAttestation(o, s.keyID, cdh[:], Policy{AppIDs: []string{app}, Roots: s.roots}, time.Now()); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}

	data := ClientData(ActionLogin, challenge)
	as := syn.assert(t, app, 1, data)
	counter, err := VerifyAssertion(as, data, key.PublicKey, app, 0, false)
	if err != nil || counter != 1 {
		t.Fatalf("assertion: %d %v", counter, err)
	}
	if _, err := VerifyAssertion(as, data, key.PublicKey, app, 1, false); !errors.Is(err, ErrInvalid) {
		t.Errorf("counter replay accepted: %v", err)
	}
	if _, err := VerifyAssertion(as, ClientData(ActionRegister, challenge), key.PublicKey, app, 0, false); !errors.Is(err, ErrInvalid) {
		t.Errorf("assertion for another action accepted: %v", err)
	}
	other, _ := newSynthetic(t, app, aaguidProduction, 0, challenge, nil)
	if _, err := VerifyAssertion(as, data, other.point, app, 0, false); !errors.Is(err, ErrInvalid) {
		t.Errorf("assertion verified with another key: %v", err)
	}
}

func TestClientData(t *testing.T) {
	if got := string(ClientData("login", "abc")); got != "opendesk-app-attest/v1\nlogin\nabc" {
		t.Fatalf("client data = %q", got)
	}
}
