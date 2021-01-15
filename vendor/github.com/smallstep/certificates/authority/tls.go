package authority

import (
	"context"
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/pem"
	"net/http"
	"time"

	"github.com/pkg/errors"
	"github.com/smallstep/certificates/authority/provisioner"
	"github.com/smallstep/certificates/db"
	"github.com/smallstep/certificates/errs"
	"go.step.sm/crypto/jose"
	"go.step.sm/crypto/keyutil"
	"go.step.sm/crypto/pemutil"
	"go.step.sm/crypto/x509util"
)

// GetTLSOptions returns the tls options configured.
func (a *Authority) GetTLSOptions() *TLSOptions {
	return a.config.TLS
}

var oidAuthorityKeyIdentifier = asn1.ObjectIdentifier{2, 5, 29, 35}
var oidSubjectKeyIdentifier = asn1.ObjectIdentifier{2, 5, 29, 14}

func withDefaultASN1DN(def *ASN1DN) provisioner.CertificateModifierFunc {
	return func(crt *x509.Certificate, opts provisioner.SignOptions) error {
		if def == nil {
			return errors.New("default ASN1DN template cannot be nil")
		}

		if len(crt.Subject.Country) == 0 && def.Country != "" {
			crt.Subject.Country = append(crt.Subject.Country, def.Country)
		}
		if len(crt.Subject.Organization) == 0 && def.Organization != "" {
			crt.Subject.Organization = append(crt.Subject.Organization, def.Organization)
		}
		if len(crt.Subject.OrganizationalUnit) == 0 && def.OrganizationalUnit != "" {
			crt.Subject.OrganizationalUnit = append(crt.Subject.OrganizationalUnit, def.OrganizationalUnit)
		}
		if len(crt.Subject.Locality) == 0 && def.Locality != "" {
			crt.Subject.Locality = append(crt.Subject.Locality, def.Locality)
		}
		if len(crt.Subject.Province) == 0 && def.Province != "" {
			crt.Subject.Province = append(crt.Subject.Province, def.Province)
		}
		if len(crt.Subject.StreetAddress) == 0 && def.StreetAddress != "" {
			crt.Subject.StreetAddress = append(crt.Subject.StreetAddress, def.StreetAddress)
		}

		return nil
	}
}

// Sign creates a signed certificate from a certificate signing request.
func (a *Authority) Sign(csr *x509.CertificateRequest, signOpts provisioner.SignOptions, extraOpts ...provisioner.SignOption) ([]*x509.Certificate, error) {
	var (
		certOptions    []x509util.Option
		certValidators []provisioner.CertificateValidator
		certModifiers  []provisioner.CertificateModifier
		certEnforcers  []provisioner.CertificateEnforcer
	)

	opts := []interface{}{errs.WithKeyVal("csr", csr), errs.WithKeyVal("signOptions", signOpts)}
	if err := csr.CheckSignature(); err != nil {
		return nil, errs.Wrap(http.StatusBadRequest, err, "authority.Sign; invalid certificate request", opts...)
	}

	// Set backdate with the configured value
	signOpts.Backdate = a.config.AuthorityConfig.Backdate.Duration

	for _, op := range extraOpts {
		switch k := op.(type) {
		// Adds new options to NewCertificate
		case provisioner.CertificateOptions:
			certOptions = append(certOptions, k.Options(signOpts)...)

		// Validate the given certificate request.
		case provisioner.CertificateRequestValidator:
			if err := k.Valid(csr); err != nil {
				return nil, errs.Wrap(http.StatusUnauthorized, err, "authority.Sign", opts...)
			}

		// Validates the unsigned certificate template.
		case provisioner.CertificateValidator:
			certValidators = append(certValidators, k)

		// Modifies a certificate before validating it.
		case provisioner.CertificateModifier:
			certModifiers = append(certModifiers, k)

		// Modifies a certificate after validating it.
		case provisioner.CertificateEnforcer:
			certEnforcers = append(certEnforcers, k)

		default:
			return nil, errs.InternalServer("authority.Sign; invalid extra option type %T", append([]interface{}{k}, opts...)...)
		}
	}

	cert, err := x509util.NewCertificate(csr, certOptions...)
	if err != nil {
		if _, ok := err.(*x509util.TemplateError); ok {
			return nil, errs.NewErr(http.StatusBadRequest, err,
				errs.WithMessage(err.Error()),
				errs.WithKeyVal("csr", csr),
				errs.WithKeyVal("signOptions", signOpts),
			)
		}
		return nil, errs.Wrap(http.StatusInternalServerError, err, "authority.Sign", opts...)
	}

	// Certificate modifiers before validation
	leaf := cert.GetCertificate()

	// Set default subject
	if err := withDefaultASN1DN(a.config.AuthorityConfig.Template).Modify(leaf, signOpts); err != nil {
		return nil, errs.Wrap(http.StatusUnauthorized, err, "authority.Sign", opts...)
	}

	for _, m := range certModifiers {
		if err := m.Modify(leaf, signOpts); err != nil {
			return nil, errs.Wrap(http.StatusUnauthorized, err, "authority.Sign", opts...)
		}
	}

	// Certificate validation.
	for _, v := range certValidators {
		if err := v.Valid(leaf, signOpts); err != nil {
			return nil, errs.Wrap(http.StatusUnauthorized, err, "authority.Sign", opts...)
		}
	}

	// Certificate modifiers after validation
	for _, m := range certEnforcers {
		if err := m.Enforce(leaf); err != nil {
			return nil, errs.Wrap(http.StatusUnauthorized, err, "authority.Sign", opts...)
		}
	}

	serverCert, err := x509util.CreateCertificate(leaf, a.x509Issuer, csr.PublicKey, a.x509Signer)
	if err != nil {
		return nil, errs.Wrap(http.StatusInternalServerError, err,
			"authority.Sign; error creating certificate", opts...)
	}

	if err = a.db.StoreCertificate(serverCert); err != nil {
		if err != db.ErrNotImplemented {
			return nil, errs.Wrap(http.StatusInternalServerError, err,
				"authority.Sign; error storing certificate in db", opts...)
		}
	}

	return []*x509.Certificate{serverCert, a.x509Issuer}, nil
}

// Renew creates a new Certificate identical to the old certificate, except
// with a validity window that begins 'now'.
func (a *Authority) Renew(oldCert *x509.Certificate) ([]*x509.Certificate, error) {
	return a.Rekey(oldCert, nil)
}

// Rekey is used for rekeying and renewing based on the public key.
// If the public key is 'nil' then it's assumed that the cert should be renewed
// using the existing public key. If the public key is not 'nil' then it's
// assumed that the cert should be rekeyed.
// For both Rekey and Renew all other attributes of the new certificate should
// match the old certificate. The exceptions are 'AuthorityKeyId' (which may
// have changed), 'SubjectKeyId' (different in case of rekey), and
// 'NotBefore/NotAfter' (the validity duration of the new certificate should be
// equal to the old one, but starting 'now').
func (a *Authority) Rekey(oldCert *x509.Certificate, pk crypto.PublicKey) ([]*x509.Certificate, error) {
	isRekey := (pk != nil)
	opts := []interface{}{errs.WithKeyVal("serialNumber", oldCert.SerialNumber.String())}

	// Check step provisioner extensions
	if err := a.authorizeRenew(oldCert); err != nil {
		return nil, errs.Wrap(http.StatusInternalServerError, err, "authority.Rekey", opts...)
	}

	// Durations
	backdate := a.config.AuthorityConfig.Backdate.Duration
	duration := oldCert.NotAfter.Sub(oldCert.NotBefore)
	now := time.Now().UTC()

	newCert := &x509.Certificate{
		Issuer:                      a.x509Issuer.Subject,
		Subject:                     oldCert.Subject,
		NotBefore:                   now.Add(-1 * backdate),
		NotAfter:                    now.Add(duration - backdate),
		KeyUsage:                    oldCert.KeyUsage,
		UnhandledCriticalExtensions: oldCert.UnhandledCriticalExtensions,
		ExtKeyUsage:                 oldCert.ExtKeyUsage,
		UnknownExtKeyUsage:          oldCert.UnknownExtKeyUsage,
		BasicConstraintsValid:       oldCert.BasicConstraintsValid,
		IsCA:                        oldCert.IsCA,
		MaxPathLen:                  oldCert.MaxPathLen,
		MaxPathLenZero:              oldCert.MaxPathLenZero,
		OCSPServer:                  oldCert.OCSPServer,
		IssuingCertificateURL:       oldCert.IssuingCertificateURL,
		PermittedDNSDomainsCritical: oldCert.PermittedDNSDomainsCritical,
		PermittedEmailAddresses:     oldCert.PermittedEmailAddresses,
		DNSNames:                    oldCert.DNSNames,
		EmailAddresses:              oldCert.EmailAddresses,
		IPAddresses:                 oldCert.IPAddresses,
		URIs:                        oldCert.URIs,
		PermittedDNSDomains:         oldCert.PermittedDNSDomains,
		ExcludedDNSDomains:          oldCert.ExcludedDNSDomains,
		PermittedIPRanges:           oldCert.PermittedIPRanges,
		ExcludedIPRanges:            oldCert.ExcludedIPRanges,
		ExcludedEmailAddresses:      oldCert.ExcludedEmailAddresses,
		PermittedURIDomains:         oldCert.PermittedURIDomains,
		ExcludedURIDomains:          oldCert.ExcludedURIDomains,
		CRLDistributionPoints:       oldCert.CRLDistributionPoints,
		PolicyIdentifiers:           oldCert.PolicyIdentifiers,
	}

	if isRekey {
		newCert.PublicKey = pk
	} else {
		newCert.PublicKey = oldCert.PublicKey
	}

	// Copy all extensions except:
	//	1. Authority Key Identifier - This one might be different if we rotate the intermediate certificate
	//					and it will cause a TLS bad certificate error.
	//	2. Subject Key Identifier, if rekey - For rekey, SubjectKeyIdentifier extension will be calculated
	//	        for the new public key by NewLeafProfilewithTemplate()
	for _, ext := range oldCert.Extensions {
		if ext.Id.Equal(oidAuthorityKeyIdentifier) {
			continue
		}
		if ext.Id.Equal(oidSubjectKeyIdentifier) && isRekey {
			newCert.SubjectKeyId = nil
			continue
		}
		newCert.ExtraExtensions = append(newCert.ExtraExtensions, ext)
	}

	serverCert, err := x509util.CreateCertificate(newCert, a.x509Issuer, newCert.PublicKey, a.x509Signer)
	if err != nil {
		return nil, errs.Wrap(http.StatusInternalServerError, err, "authority.Rekey", opts...)
	}

	if err = a.db.StoreCertificate(serverCert); err != nil {
		if err != db.ErrNotImplemented {
			return nil, errs.Wrap(http.StatusInternalServerError, err, "authority.Rekey; error storing certificate in db", opts...)
		}
	}

	return []*x509.Certificate{serverCert, a.x509Issuer}, nil
}

// RevokeOptions are the options for the Revoke API.
type RevokeOptions struct {
	Serial      string
	Reason      string
	ReasonCode  int
	PassiveOnly bool
	MTLS        bool
	Crt         *x509.Certificate
	OTT         string
}

// Revoke revokes a certificate.
//
// NOTE: Only supports passive revocation - prevent existing certificates from
// being renewed.
//
// TODO: Add OCSP and CRL support.
func (a *Authority) Revoke(ctx context.Context, revokeOpts *RevokeOptions) error {
	opts := []interface{}{
		errs.WithKeyVal("serialNumber", revokeOpts.Serial),
		errs.WithKeyVal("reasonCode", revokeOpts.ReasonCode),
		errs.WithKeyVal("reason", revokeOpts.Reason),
		errs.WithKeyVal("passiveOnly", revokeOpts.PassiveOnly),
		errs.WithKeyVal("MTLS", revokeOpts.MTLS),
		errs.WithKeyVal("context", provisioner.MethodFromContext(ctx).String()),
	}
	if revokeOpts.MTLS {
		opts = append(opts, errs.WithKeyVal("certificate", base64.StdEncoding.EncodeToString(revokeOpts.Crt.Raw)))
	} else {
		opts = append(opts, errs.WithKeyVal("token", revokeOpts.OTT))
	}

	rci := &db.RevokedCertificateInfo{
		Serial:     revokeOpts.Serial,
		ReasonCode: revokeOpts.ReasonCode,
		Reason:     revokeOpts.Reason,
		MTLS:       revokeOpts.MTLS,
		RevokedAt:  time.Now().UTC(),
	}

	var (
		p   provisioner.Interface
		err error
	)
	// If not mTLS then get the TokenID of the token.
	if !revokeOpts.MTLS {
		token, err := jose.ParseSigned(revokeOpts.OTT)
		if err != nil {
			return errs.Wrap(http.StatusUnauthorized, err,
				"authority.Revoke; error parsing token", opts...)
		}

		// Get claims w/out verification.
		var claims Claims
		if err = token.UnsafeClaimsWithoutVerification(&claims); err != nil {
			return errs.Wrap(http.StatusUnauthorized, err, "authority.Revoke", opts...)
		}

		// This method will also validate the audiences for JWK provisioners.
		var ok bool
		p, ok = a.provisioners.LoadByToken(token, &claims.Claims)
		if !ok {
			return errs.InternalServer("authority.Revoke; provisioner not found", opts...)
		}
		rci.TokenID, err = p.GetTokenID(revokeOpts.OTT)
		if err != nil {
			return errs.Wrap(http.StatusInternalServerError, err,
				"authority.Revoke; could not get ID for token")
		}
		opts = append(opts, errs.WithKeyVal("tokenID", rci.TokenID))
	} else {
		// Load the Certificate provisioner if one exists.
		p, err = a.LoadProvisionerByCertificate(revokeOpts.Crt)
		if err != nil {
			return errs.Wrap(http.StatusUnauthorized, err,
				"authority.Revoke: unable to load certificate provisioner", opts...)
		}
	}
	rci.ProvisionerID = p.GetID()
	opts = append(opts, errs.WithKeyVal("provisionerID", rci.ProvisionerID))

	if provisioner.MethodFromContext(ctx) == provisioner.SSHRevokeMethod {
		err = a.db.RevokeSSH(rci)
	} else { // default to revoke x509
		err = a.db.Revoke(rci)
	}
	switch err {
	case nil:
		return nil
	case db.ErrNotImplemented:
		return errs.NotImplemented("authority.Revoke; no persistence layer configured", opts...)
	case db.ErrAlreadyExists:
		return errs.BadRequest("authority.Revoke; certificate with serial "+
			"number %s has already been revoked", append([]interface{}{rci.Serial}, opts...)...)
	default:
		return errs.Wrap(http.StatusInternalServerError, err, "authority.Revoke", opts...)
	}
}

// GetTLSCertificate creates a new leaf certificate to be used by the CA HTTPS server.
func (a *Authority) GetTLSCertificate() (*tls.Certificate, error) {
	fatal := func(err error) (*tls.Certificate, error) {
		return nil, errs.Wrap(http.StatusInternalServerError, err, "authority.GetTLSCertificate")
	}

	// Generate default key.
	priv, err := keyutil.GenerateDefaultKey()
	if err != nil {
		return fatal(err)
	}
	signer, ok := priv.(crypto.Signer)
	if !ok {
		return fatal(errors.New("private key is not a crypto.Signer"))
	}

	// Create initial certificate request.
	cr, err := x509util.CreateCertificateRequest("Step Online CA", a.config.DNSNames, signer)
	if err != nil {
		return fatal(err)
	}

	// Generate certificate template directly from the certificate request.
	template, err := x509util.NewCertificate(cr)
	if err != nil {
		return fatal(err)
	}

	// Get x509 certificate template, set validity and sign it.
	now := time.Now()
	certTpl := template.GetCertificate()
	certTpl.NotBefore = now.Add(-1 * time.Minute)
	certTpl.NotAfter = now.Add(24 * time.Hour)

	cert, err := x509util.CreateCertificate(certTpl, a.x509Issuer, cr.PublicKey, a.x509Signer)
	if err != nil {
		return fatal(err)
	}

	// Generate PEM blocks to create tls.Certificate
	crtPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	})
	intermediatePEM, err := pemutil.Serialize(a.x509Issuer)
	if err != nil {
		return fatal(err)
	}
	keyPEM, err := pemutil.Serialize(priv)
	if err != nil {
		return fatal(err)
	}

	tlsCrt, err := tls.X509KeyPair(append(crtPEM, pem.EncodeToMemory(intermediatePEM)...), pem.EncodeToMemory(keyPEM))
	if err != nil {
		return fatal(err)
	}
	// Set leaf certificate
	tlsCrt.Leaf = cert
	return &tlsCrt, nil
}
