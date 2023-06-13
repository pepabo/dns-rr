package endpoint

type Endpoint struct {
	// The Dns Name
	DnsName string
	// The type of DNS Record
	Class string

	// The value of DNS Record
	Rdata string

	// The TTL of DNS Record
	Ttl int64

	// The Id of DNS Record
	Id string

	// The owner of DNS Record
	ResourceOwner string

	// The Record weighte
	Weight *int64

	// The flag of Alias Record
	IsAlias bool

	// The alias target of DNS Record
	AliasTarget AliasOpts
}

type AliasOpts struct {
	DnsName                   string
	HostedZoneId              string
	EvaluateAliasTargetHealth bool
}
