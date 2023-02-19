package provider

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
	dnsv1alpha1 "github.com/ch1aki/dns-rr/api/v1alpha1"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	gomock "github.com/golang/mock/gomock"
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
		{
			name: "no diff in weighted record",
			args: args{
				owners:   []string{"test"},
				zoneName: "example.com",
				desiredEp: endpoint{
					dnsName: "test.example.com.",
					class:   "A",
					rdata:   "198.51.100.1",
					ttl:     300,
					weight:  aws.Int64(10),
					id:      "weighted-record",
				},
				acutualEps: map[string]endpoint{
					"test": {
						dnsName: "test.example.com.",
						class:   "A",
						rdata:   "198.51.100.1",
						ttl:     300,
						weight:  aws.Int64(10),
						id:      "weighted-record",
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
				desiredEp: endpoint{
					dnsName: "test.example.com.",
					class:   "A",
					rdata:   "198.51.100.1",
					ttl:     300,
					weight:  aws.Int64(10),
					id:      "weighted-record",
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
				desiredEp: endpoint{
					dnsName: "test.example.com.",
					class:   "A",
					weight:  aws.Int64(10),
					id:      "weighted-record",
					isAlias: true,
					aliasTarget: aliasOpts{
						dnsName:                   "target.example.com.",
						hostedZoneId:              "Z0123456789ABCDEFGHIJ",
						evaluateAliasTargetHealth: true,
					},
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
						id:      *aws.String("weighted-record"),
						weight:  aws.Int64(200),
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
		beforeDo func() (Route53API, *gomock.Controller)
		want     map[string]endpoint
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
			beforeDo: func() (Route53API, *gomock.Controller) {
				controller := gomock.NewController(t)
				r53api := NewMockRoute53API(controller)
				r53api.EXPECT().ListResourceRecordSets(
					context.TODO(),
					&route53.ListResourceRecordSetsInput{
						HostedZoneId:    aws.String("Z0123456789ABCDEFGHIJ"),
						StartRecordName: aws.String("test.example.com."),
					},
				).Return(
					&route53.ListResourceRecordSetsOutput{
						ResourceRecordSets: []types.ResourceRecordSet{
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
						},
					},
					nil,
				).Times(1)
				return r53api, controller
			},
			want: map[string]endpoint{
				"test": {
					dnsName: "test.example.com.",
					class:   "A",
					rdata:   "198.51.100.1",
					ttl:     300,
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
			beforeDo: func() (Route53API, *gomock.Controller) {
				controller := gomock.NewController(t)
				r53api := NewMockRoute53API(controller)
				r53api.EXPECT().ListResourceRecordSets(
					context.TODO(),
					&route53.ListResourceRecordSetsInput{
						HostedZoneId:    aws.String("Z0123456789ABCDEFGHIJ"),
						StartRecordName: aws.String("alias.example.com."),
					},
				).Return(
					&route53.ListResourceRecordSetsOutput{
						ResourceRecordSets: []types.ResourceRecordSet{
							{
								Name: aws.String("alias.example.com."),
								Type: types.RRTypeA,
								AliasTarget: &types.AliasTarget{
									DNSName:              aws.String("test.example.com."),
									HostedZoneId:         aws.String("Z0123456789ABCDEFGHIJ"),
									EvaluateTargetHealth: true,
								},
							},
						},
					},
					nil,
				).Times(1)
				return r53api, controller
			},
			want: map[string]endpoint{
				"alias": {
					dnsName: "alias.example.com.",
					class:   "A",
					aliasTarget: aliasOpts{
						dnsName:                   "test.example.com.",
						hostedZoneId:              "Z0123456789ABCDEFGHIJ",
						evaluateAliasTargetHealth: true,
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
			beforeDo: func() (Route53API, *gomock.Controller) {
				controller := gomock.NewController(t)
				r53api := NewMockRoute53API(controller)
				r53api.EXPECT().ListResourceRecordSets(
					context.TODO(),
					&route53.ListResourceRecordSetsInput{
						HostedZoneId:    aws.String("Z0123456789ABCDEFGHIJ"),
						StartRecordName: aws.String("weighted.example.com."),
					},
				).Return(
					&route53.ListResourceRecordSetsOutput{
						ResourceRecordSets: []types.ResourceRecordSet{
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
						},
					},
					nil,
				).Times(1)
				return r53api, controller
			},
			want: map[string]endpoint{
				"weighted": {
					dnsName: "weighted.example.com.",
					class:   "A",
					rdata:   "198.51.100.1",
					ttl:     300,
					id:      "weighted-test",
					weight:  aws.Int64(10),
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, controller := tt.beforeDo()
			defer controller.Finish()
			got, err := records(context.TODO(), client, tt.args.zoneId, tt.args.zoneName, tt.args.owners, tt.args.recordType, tt.args.id)
			opts := cmp.AllowUnexported(endpoint{}, aliasOpts{})
			if diff := cmp.Diff(got, tt.want, opts); diff != "" {
				t.Errorf("differs: (-got +want)\n%s", diff)
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("Ensure() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConverge(t *testing.T) {
	type args struct {
		zoneId   string
		zoneName string
		owners   []string
		rrSpec   dnsv1alpha1.ResourceRecordSpec
	}
	tests := []struct {
		name          string
		args          args
		beforeDo      func() (Route53API, *gomock.Controller)
		recordsResult map[string]endpoint
		diffResult    []types.Change
		wantErr       bool
	}{
		{
			name: "execute",
			args: args{
				zoneId:   "Z0123456789ABCDEFGHIJ",
				zoneName: "example.com",
				owners:   []string{"test"},
				rrSpec:   dnsv1alpha1.ResourceRecordSpec{DryRun: false},
			},
			beforeDo: func() (Route53API, *gomock.Controller) {
				controller := gomock.NewController(t)
				r53api := NewMockRoute53API(controller)
				r53api.EXPECT().ChangeResourceRecordSets(
					context.TODO(),
					&route53.ChangeResourceRecordSetsInput{
						HostedZoneId: aws.String("Z0123456789ABCDEFGHIJ"),
						ChangeBatch: &types.ChangeBatch{
							Changes: make([]types.Change, 1),
						},
					},
				).Return(
					&route53.ChangeResourceRecordSetsOutput{},
					nil,
				).Times(1)
				return r53api, controller
			},
			diffResult: make([]types.Change, 1),
			wantErr:    false,
		},
		{
			name: "dry-run",
			args: args{
				zoneId:   "Z0123456789ABCDEFGHIJ",
				zoneName: "example.com",
				owners:   []string{"test"},
				rrSpec:   dnsv1alpha1.ResourceRecordSpec{DryRun: true},
			},
			beforeDo: func() (Route53API, *gomock.Controller) {
				controller := gomock.NewController(t)
				r53api := NewMockRoute53API(controller)
				return r53api, controller
			},
			diffResult: make([]types.Change, 1),
			wantErr:    false,
		},
		{
			name: "no diff",
			args: args{
				zoneId:   "Z0123456789ABCDEFGHIJ",
				zoneName: "example.com",
				owners:   []string{"test"},
				rrSpec:   dnsv1alpha1.ResourceRecordSpec{DryRun: false},
			},
			beforeDo: func() (Route53API, *gomock.Controller) {
				controller := gomock.NewController(t)
				r53api := NewMockRoute53API(controller)
				return r53api, controller
			},
			diffResult: make([]types.Change, 0),
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, controller := tt.beforeDo()
			defer controller.Finish()
			p := Route53Provider{
				client: c,
				diff: func(owners []string, zoneName string, desiredEp endpoint, actualEps map[string]endpoint) []types.Change {
					return tt.diffResult
				},
				recordsFx: func(ctx context.Context, client Route53API, zoneId string, zoneName string, owners []string, recordType string, id *string) (map[string]endpoint, error) {
					return tt.recordsResult, nil
				},
			}
			err := p.Converge(context.TODO(), tt.args.zoneId, tt.args.zoneName, tt.args.owners, tt.args.rrSpec)
			//opts := cmp.AllowUnexported(endpoint{}, aliasOpts{})
			if (err != nil) != tt.wantErr {
				t.Errorf("Ensure() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
