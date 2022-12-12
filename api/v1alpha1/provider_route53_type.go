package v1alpha1

type AWSAuth struct {
	SecretRef *AWSAuthSecretRef `json:"secretRef,omitempty"`
}

type AWSAuthSecretRef struct {
	// The AccessKeyID is used for authentication
	AccessKeyID SecretKeySelector `json:"accessKeyIDSecretRef,omitempty"`

	// The SecretAccessKey is used for authentication
	SecretAccessKey SecretKeySelector `json:"secretAccessKeySecretRef,omitempty"`
}

type Route53Provider struct {
	HostedZoneID string `json:"hostedZoneID"`

	// +optional
	Region string `json:"region"`

	// +optional
	Auth AWSAuth `json:"auth"`
}
