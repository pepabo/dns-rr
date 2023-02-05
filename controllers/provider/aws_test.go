package provider

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestBuildFQDN(t *testing.T) {
	tests := []struct {
		name  string
		owner string
		zone  string
		want  string
	}{
		{
			name:  "without root node",
			owner: "test",
			zone:  "example.com",
			want:  "test.example.com.",
		},
		{
			name:  "with root node",
			owner: "test",
			zone:  "example.com.",
			want:  "test.example.com.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildFQDN(tt.owner, tt.zone); got != tt.want {
				t.Errorf("buildFQDN() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDiff(t *testing.T) {
	type args struct {
		owners     []string
		zoneName   string
		desiredEp  endpoint
		acutualEps map[string]endpoint
	}
	tests := []struct {
		name string
		args args
		want []types.Change
	}{
		{
			name: "no diff",
			args: args{
				owners:   []string{"test"},
				zoneName: "example.com",
				desiredEp: endpoint{
					dnsName: "test.example.com.",
					class:   "A",
					rdata:   "192.0.2.1",
					ttl:     300,
				},
				acutualEps: map[string]endpoint{
					"test": {
						dnsName: "test.example.com.",
						class:   "A",
						rdata:   "192.0.2.1",
						ttl:     300,
					},
				},
			},
			want: make([]types.Change, 0),
		},
		{
			name: "no record",
			args: args{
				owners:   []string{"test"},
				zoneName: "example.com",
				desiredEp: endpoint{
					dnsName: "test.example.com.",
					class:   "A",
					rdata:   "192.0.2.1",
					ttl:     300,
				},
				acutualEps: map[string]endpoint{},
			},
			want: []types.Change{
				{
					ResourceRecordSet: &types.ResourceRecordSet{
						Name: aws.String("test.example.com."),
						Type: types.RRTypeA,
						TTL:  aws.Int64(300),
						ResourceRecords: []types.ResourceRecord{
							{Value: aws.String("192.0.2.1")},
						},
					},
					Action: types.ChangeActionCreate,
				},
			},
		},
		{
			name: "diff in rdata",
			args: args{
				owners:   []string{"test"},
				zoneName: "example.com",
				desiredEp: endpoint{
					dnsName: "test.example.com.",
					class:   "A",
					rdata:   "192.0.2.1",
					ttl:     300,
				},
				acutualEps: map[string]endpoint{
					"test": {
						dnsName: "test.example.com.",
						class:   "A",
						rdata:   "198.51.100.1",
						ttl:     300,
					},
				},
			},
			want: []types.Change{
				{
					ResourceRecordSet: &types.ResourceRecordSet{
						Name: aws.String("test.example.com."),
						Type: types.RRTypeA,
						TTL:  aws.Int64(300),
						ResourceRecords: []types.ResourceRecord{
							{Value: aws.String("192.0.2.1")},
						},
					},
					Action: types.ChangeActionUpsert,
				},
			},
		},
		{
			name: "diff in alias target",
			args: args{
				owners:   []string{"test"},
				zoneName: "example.com",
				desiredEp: endpoint{
					dnsName: "test.example.com.",
					class:   "A",
					aliasTarget: aliasOpts{
						dnsName:                   "target.example.com.",
						hostedZoneId:              "Z0123456789ABCDEFGHIJ",
						evaluateAliasTargetHealth: true,
					},
					isAlias: true,
				},
				acutualEps: map[string]endpoint{
					"test": {
						dnsName: "test.example.com.",
						class:   "A",
						aliasTarget: aliasOpts{
							dnsName:                   "wrong.example.com.",
							hostedZoneId:              "Z0987654321ZYXVUTSRQP",
							evaluateAliasTargetHealth: false,
						},
						isAlias: true,
					},
				},
			},
			want: []types.Change{
				{
					ResourceRecordSet: &types.ResourceRecordSet{
						Name: aws.String("test.example.com."),
						Type: types.RRTypeA,
						AliasTarget: &types.AliasTarget{
							DNSName:              aws.String("target.example.com."),
							HostedZoneId:         aws.String("Z0123456789ABCDEFGHIJ"),
							EvaluateTargetHealth: true,
						},
					},
					Action: types.ChangeActionUpsert,
				},
			},
		},
		{
			name: "diff in record type",
			args: args{
				owners:   []string{"test"},
				zoneName: "example.com",
				desiredEp: endpoint{
					dnsName: "test.example.com.",
					class:   "A",
					aliasTarget: aliasOpts{
						dnsName:                   "target.example.com.",
						hostedZoneId:              "Z0123456789ABCDEFGHIJ",
						evaluateAliasTargetHealth: true,
					},
					isAlias: true,
				},
				acutualEps: map[string]endpoint{
					"test": {
						dnsName: "test.example.com.",
						class:   "A",
						rdata:   "198.51.100.1",
						ttl:     300,
					},
				},
			},
			want: []types.Change{
				{
					ResourceRecordSet: &types.ResourceRecordSet{
						Name: aws.String("test.example.com."),
						Type: types.RRTypeA,
						AliasTarget: &types.AliasTarget{
							DNSName:              aws.String("target.example.com."),
							HostedZoneId:         aws.String("Z0123456789ABCDEFGHIJ"),
							EvaluateTargetHealth: true,
						},
					},
					Action: types.ChangeActionUpsert,
				},
			},
		},
		{
			name: "diff in record class",
			args: args{
				owners:   []string{"test"},
				zoneName: "example.com",
				desiredEp: endpoint{
					dnsName: "test.example.com.",
					class:   "TXT",
					rdata:   "test",
					ttl:     300,
				},
				acutualEps: map[string]endpoint{
					"test": {
						dnsName: "test.example.com.",
						class:   "A",
						rdata:   "198.51.100.1",
						ttl:     300,
					},
				},
			},
			want: []types.Change{
				{
					ResourceRecordSet: &types.ResourceRecordSet{
						Name: aws.String("test.example.com."),
						Type: types.RRTypeTxt,
						TTL:  aws.Int64(300),
						ResourceRecords: []types.ResourceRecord{
							{Value: aws.String("test")},
						},
					},
					Action: types.ChangeActionUpsert,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := diff(tt.args.owners, tt.args.zoneName, tt.args.desiredEp, tt.args.acutualEps)

			// ignore option for enexported field (noSmithyDocumentSerde)
			opts := cmpopts.IgnoreUnexported(types.Change{}, types.ResourceRecordSet{}, types.ResourceRecord{}, types.AliasTarget{})
			if diff := cmp.Diff(got, tt.want, opts); diff != "" {
				t.Errorf("differs: (-got +want)\n%s", diff)
			}
		})
	}
}