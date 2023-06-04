package provider

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/ch1aki/dns-rr/controllers/endpoint"
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
		desiredEp  endpoint.Endpoint
		acutualEps map[string]endpoint.Endpoint
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
				desiredEp: endpoint.Endpoint{
					DnsName: "test.example.com.",
					Class:   "A",
					Rdata:   "192.0.2.1",
					Ttl:     300,
				},
				acutualEps: map[string]endpoint.Endpoint{
					"test": {
						DnsName: "test.example.com.",
						Class:   "A",
						Rdata:   "192.0.2.1",
						Ttl:     300,
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
				desiredEp: endpoint.Endpoint{
					DnsName: "test.example.com.",
					Class:   "A",
					Rdata:   "192.0.2.1",
					Ttl:     300,
				},
				acutualEps: map[string]endpoint.Endpoint{},
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
				desiredEp: endpoint.Endpoint{
					DnsName: "test.example.com.",
					Class:   "A",
					Rdata:   "192.0.2.1",
					Ttl:     300,
				},
				acutualEps: map[string]endpoint.Endpoint{
					"test": {
						DnsName: "test.example.com.",
						Class:   "A",
						Rdata:   "198.51.100.1",
						Ttl:     300,
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
				desiredEp: endpoint.Endpoint{
					DnsName: "test.example.com.",
					Class:   "A",
					AliasTarget: endpoint.AliasOpts{
						DnsName:                   "target.example.com.",
						HostedZoneId:              "Z0123456789ABCDEFGHIJ",
						EvaluateAliasTargetHealth: true,
					},
					IsAlias: true,
				},
				acutualEps: map[string]endpoint.Endpoint{
					"test": {
						DnsName: "test.example.com.",
						Class:   "A",
						AliasTarget: endpoint.AliasOpts{
							DnsName:                   "wrong.example.com.",
							HostedZoneId:              "Z0987654321ZYXVUTSRQP",
							EvaluateAliasTargetHealth: false,
						},
						IsAlias: true,
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
				desiredEp: endpoint.Endpoint{
					DnsName: "test.example.com.",
					Class:   "A",
					AliasTarget: endpoint.AliasOpts{
						DnsName:                   "target.example.com.",
						HostedZoneId:              "Z0123456789ABCDEFGHIJ",
						EvaluateAliasTargetHealth: true,
					},
					IsAlias: true,
				},
				acutualEps: map[string]endpoint.Endpoint{
					"test": {
						DnsName: "test.example.com.",
						Class:   "A",
						Rdata:   "198.51.100.1",
						Ttl:     300,
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
				desiredEp: endpoint.Endpoint{
					DnsName: "test.example.com.",
					Class:   "TXT",
					Rdata:   "test",
					Ttl:     300,
				},
				acutualEps: map[string]endpoint.Endpoint{
					"test": {
						DnsName: "test.example.com.",
						Class:   "A",
						Rdata:   "198.51.100.1",
						Ttl:     300,
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
		{
			name: "no diff in weighted record",
			args: args{
				owners:   []string{"test"},
				zoneName: "example.com",
				desiredEp: endpoint.Endpoint{
					DnsName: "test.example.com.",
					Class:   "A",
					Rdata:   "198.51.100.1",
					Ttl:     300,
					Weight:  aws.Int64(10),
					Id:      "weighted-record",
				},
				acutualEps: map[string]endpoint.Endpoint{
					"test": {
						DnsName: "test.example.com.",
						Class:   "A",
						Rdata:   "198.51.100.1",
						Ttl:     300,
						Weight:  aws.Int64(10),
						Id:      "weighted-record",
					},
				},
			},
			want: make([]types.Change, 0),
		},
		{
			name: "no weighted record",
			args: args{
				owners:   []string{"test"},
				zoneName: "example.com",
				desiredEp: endpoint.Endpoint{
					DnsName: "test.example.com.",
					Class:   "A",
					Rdata:   "198.51.100.1",
					Ttl:     300,
					Weight:  aws.Int64(10),
					Id:      "weighted-record",
				},
				acutualEps: map[string]endpoint.Endpoint{},
			},
			want: []types.Change{
				{
					ResourceRecordSet: &types.ResourceRecordSet{
						Name: aws.String("test.example.com."),
						Type: types.RRTypeA,
						TTL:  aws.Int64(300),
						ResourceRecords: []types.ResourceRecord{
							{Value: aws.String("198.51.100.1")},
						},
						Weight:        aws.Int64(10),
						SetIdentifier: aws.String("weighted-record"),
					},
					Action: types.ChangeActionCreate,
				},
			},
		},
		{
			name: "diff in alias weighted record",
			args: args{
				owners:   []string{"test"},
				zoneName: "example.com",
				desiredEp: endpoint.Endpoint{
					DnsName: "test.example.com.",
					Class:   "A",
					Weight:  aws.Int64(10),
					Id:      "weighted-record",
					IsAlias: true,
					AliasTarget: endpoint.AliasOpts{
						DnsName:                   "target.example.com.",
						HostedZoneId:              "Z0123456789ABCDEFGHIJ",
						EvaluateAliasTargetHealth: true,
					},
				},
				acutualEps: map[string]endpoint.Endpoint{
					"test": {
						DnsName: "test.example.com.",
						Class:   "A",
						AliasTarget: endpoint.AliasOpts{
							DnsName:                   "wrong.example.com.",
							HostedZoneId:              "Z0987654321ZYXVUTSRQP",
							EvaluateAliasTargetHealth: false,
						},
						IsAlias: true,
						Id:      *aws.String("weighted-record"),
						Weight:  aws.Int64(200),
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
						SetIdentifier: aws.String("weighted-record"),
						Weight:        aws.Int64(10),
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

func TestRecords(t *testing.T) {
	type args struct {
		zoneId     string
		zoneName   string
		owners     []string
		recordType string
		id         *string
	}
	tests := []struct {
		name     string
		args     args
		beforeDo func() Route53Provider
		want     map[string]endpoint.Endpoint
		wantErr  bool
	}{
		{
			name: "get matched record",
			args: args{
				zoneId:     "Z0123456789ABCDEFGHIJ",
				zoneName:   "example.com",
				owners:     []string{"test"},
				recordType: "A",
			},
			beforeDo: func() Route53Provider {
				key := "exampleNS/example"
				cache := map[string][]types.ResourceRecordSet{key: {
					{
						Name:            aws.String("test.example.com."),
						Type:            types.RRTypeA,
						ResourceRecords: []types.ResourceRecord{{Value: aws.String("198.51.100.1")}},
						TTL:             aws.Int64(300),
					},
					{
						Name:            aws.String("test.example.com.example.com."),
						Type:            types.RRTypeTxt,
						ResourceRecords: []types.ResourceRecord{{Value: aws.String("expected ignore")}},
						TTL:             aws.Int64(600),
					},
				}}

				return Route53Provider{hostedZoneId: "Z0123456789ABCDEFGHIJ", cacheKey: key, providerZoneCache: cache}
			},
			want: map[string]endpoint.Endpoint{
				"test": {
					DnsName: "test.example.com.",
					Class:   "A",
					Rdata:   "198.51.100.1",
					Ttl:     300,
				},
			},
			wantErr: false,
		},
		{
			name: "get matched alias record",
			args: args{
				zoneId:     "Z0123456789ABCDEFGHIJ",
				zoneName:   "example.com",
				owners:     []string{"alias"},
				recordType: "A",
			},
			beforeDo: func() Route53Provider {
				key := "exampleNS/example"
				cache := map[string][]types.ResourceRecordSet{key: {
					{
						Name: aws.String("alias.example.com."),
						Type: types.RRTypeA,
						AliasTarget: &types.AliasTarget{
							DNSName:              aws.String("test.example.com."),
							HostedZoneId:         aws.String("Z0123456789ABCDEFGHIJ"),
							EvaluateTargetHealth: true,
						},
					},
				}}

				return Route53Provider{hostedZoneId: "Z0123456789ABCDEFGHIJ", cacheKey: key, providerZoneCache: cache}
			},
			want: map[string]endpoint.Endpoint{
				"alias": {
					DnsName: "alias.example.com.",
					Class:   "A",
					AliasTarget: endpoint.AliasOpts{
						DnsName:                   "test.example.com.",
						HostedZoneId:              "Z0123456789ABCDEFGHIJ",
						EvaluateAliasTargetHealth: true,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "get weighted records",
			args: args{
				zoneId:     "Z0123456789ABCDEFGHIJ",
				zoneName:   "example.com",
				owners:     []string{"weighted"},
				recordType: "A",
				id:         aws.String("weighted-test"),
			},
			beforeDo: func() Route53Provider {
				key := "exampleNS/example"
				cache := map[string][]types.ResourceRecordSet{key: {
					{
						Name:            aws.String("weighted.example.com."),
						Type:            types.RRTypeA,
						ResourceRecords: []types.ResourceRecord{{Value: aws.String("198.51.100.1")}},
						TTL:             aws.Int64(300),
						SetIdentifier:   aws.String("weighted-test"),
						Weight:          aws.Int64(10),
					},
					{
						Name:            aws.String("weighted.example.com."),
						Type:            types.RRTypeA,
						ResourceRecords: []types.ResourceRecord{{Value: aws.String("198.51.100.2")}},
						TTL:             aws.Int64(300),
						SetIdentifier:   aws.String("managed-by-other"),
						Weight:          aws.Int64(20),
					},
				}}
				return Route53Provider{hostedZoneId: "Z0123456789ABCDEFGHIJ", cacheKey: key, providerZoneCache: cache}
			},
			want: map[string]endpoint.Endpoint{
				"weighted": {
					DnsName: "weighted.example.com.",
					Class:   "A",
					Rdata:   "198.51.100.1",
					Ttl:     300,
					Id:      "weighted-test",
					Weight:  aws.Int64(10),
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.beforeDo()
			got, err := p.records(context.TODO(), tt.args.zoneId, tt.args.zoneName, tt.args.owners, tt.args.recordType, tt.args.id)
			opts := cmp.AllowUnexported(endpoint.AliasOpts{}, endpoint.AliasOpts{})
			if diff := cmp.Diff(got, tt.want, opts); diff != "" {
				t.Errorf("differs: (-got +want)\n%s", diff)
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("Ensure() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
