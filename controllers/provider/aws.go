package provider

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/pkg/errors"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dnsv1alpha1 "github.com/ch1aki/dns-rr/api/v1alpha1"
	"github.com/ch1aki/dns-rr/controllers/endpoint"
)

const recordOwnerPrefix = "dns-rr-owner: "

type Route53Provider struct {
	hostedZoneId string
	client       Route53API
}

func (r Route53Provider) NewClient(ctx context.Context, provider *dnsv1alpha1.Provider, c client.Client) (*Route53Provider, error) {
	var optFns []func(*config.LoadOptions) error

	// secret ref option
	if provider.Spec.Route53.Auth.SecretRef != nil {
		cred, err := credFromSecretRef(ctx, provider, c)
		if err != nil {
			return nil, err
		}
		optFns = append(optFns, config.WithCredentialsProvider(cred))
	}

	// region option
	if region := provider.Spec.Route53.Region; region == "" {
		return nil, fmt.Errorf("route53 provider require region")
	} else {
		optFns = append(optFns, config.WithRegion(region))
	}

	cfg, err := config.LoadDefaultConfig(ctx, optFns...)
	if err != nil {
		return nil, fmt.Errorf("load config error: %w", err)
	}

	return &Route53Provider{
		hostedZoneId: provider.Spec.Route53.HostedZoneID,
		client:       route53.NewFromConfig(cfg),
	}, nil
}

func (p Route53Provider) Converge(ctx context.Context, zoneId string, zoneName string, owners []string, rrSpec dnsv1alpha1.ResourceRecordSpec) error {
	// build desired endpoint
	desired := endpoint.Endpoint{
		Class: rrSpec.Class,
		Ttl:   int64(rrSpec.Ttl),
	}
	if rrSpec.IsAlias {
		desired.IsAlias = true
		desired.AliasTarget = endpoint.AliasOpts{
			DnsName:                   rrSpec.AliasTarget.Record,
			HostedZoneId:              rrSpec.AliasTarget.HostedZoneID,
			EvaluateAliasTargetHealth: rrSpec.AliasTarget.EvaluateTargetHealth,
		}
	} else {
		desired.Rdata = rrSpec.Rdata
	}

	if rrSpec.Weight != nil {
		desired.Weight = rrSpec.Weight
	}
	if rrSpec.Id != nil {
		desired.Id = *rrSpec.Id
	}

	// get actual endpoints
	currentRecords, err := p.records(ctx, zoneId, zoneName, owners, rrSpec.Class, rrSpec.Id)
	if err != nil {
		return err
	}

	// evalute differences
	changes := diff(owners, zoneName, desired, currentRecords)

	// converge
	if 0 < len(changes) {
		changeRrsInput := route53.ChangeResourceRecordSetsInput{
			HostedZoneId: &zoneId,
			ChangeBatch: &types.ChangeBatch{
				Changes: changes,
			},
		}
		if _, err := p.client.ChangeResourceRecordSets(ctx, &changeRrsInput); err != nil {
			return errors.Wrapf(err, "failes to change resource records set for zone %s", zoneId)
		}
	}
	return nil
}

func diff(owners []string, zoneName string, desiredEp endpoint.Endpoint, actualEps map[string]endpoint.Endpoint) []types.Change {
	changes := make([]types.Change, 0)

	for _, owner := range owners {
		// build changes
		fqdn := buildFQDN(owner, zoneName)
		desiredEp.DnsName = fqdn
		var c types.Change
		if desiredEp.IsAlias {
			c = types.Change{
				ResourceRecordSet: &types.ResourceRecordSet{
					Name: aws.String(fqdn),
					Type: types.RRType(desiredEp.Class),
					AliasTarget: &types.AliasTarget{
						DNSName:              &desiredEp.AliasTarget.DnsName,
						HostedZoneId:         &desiredEp.AliasTarget.HostedZoneId,
						EvaluateTargetHealth: desiredEp.AliasTarget.EvaluateAliasTargetHealth,
					},
				},
			}
		} else {
			c = types.Change{
				ResourceRecordSet: &types.ResourceRecordSet{
					Name:            aws.String(fqdn),
					Type:            types.RRType(desiredEp.Class),
					TTL:             &desiredEp.Ttl,
					ResourceRecords: []types.ResourceRecord{{Value: &desiredEp.Rdata}},
				},
			}
		}

		// weighted record
		if desiredEp.Weight != nil {
			c.ResourceRecordSet.SetIdentifier = &desiredEp.Id
			c.ResourceRecordSet.Weight = desiredEp.Weight
		}

		// evaluate difference
		if _, exist := actualEps[owner]; !exist {
			// レコードが存在しなかった場合
			c.Action = types.ChangeActionCreate
			changes = append(changes, c)
		} else if !reflect.DeepEqual(desiredEp, actualEps[owner]) {
			// 値が異なる場合
			c.Action = types.ChangeActionUpsert
			changes = append(changes, c)
		}
		// TODO: delete record when delete record definition
	}
	return changes
}

func (p *Route53Provider) records(ctx context.Context, zoneId string, zoneName string, owners []string, recordType string, id *string) (map[string]endpoint.Endpoint, error) {
	endpoints := make(map[string]endpoint.Endpoint, len(owners))
	for _, owner := range owners {
		fqdn := buildFQDN(owner, zoneName)
		// owner から始まるrecordsetをリスト
		params := &route53.ListResourceRecordSetsInput{
			HostedZoneId:    &zoneId,
			StartRecordName: aws.String(fqdn),
		}
		output, err := p.client.ListResourceRecordSets(ctx, params)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to list resource records sets for zone %s", zoneId)
		}

		// 一致するレコードを検索
		ep := endpoint.Endpoint{
			DnsName: fqdn,
			Class:   recordType,
		}
		for _, r := range output.ResourceRecordSets {
			if *r.Name == fqdn {
				if r.Type == types.RRType(recordType) {
					// Set rdata or alias target value
					if r.AliasTarget != nil {
						ep.AliasTarget.DnsName = *r.AliasTarget.DNSName
						ep.AliasTarget.HostedZoneId = *r.AliasTarget.HostedZoneId
						ep.AliasTarget.EvaluateAliasTargetHealth = *&r.AliasTarget.EvaluateTargetHealth
					} else {
						// TODO: multi value レコードを考慮する
						ep.Rdata = *r.ResourceRecords[0].Value

						// set ttl
						ep.Ttl = *r.TTL
					}

					// 一致するIDのレコードだけを対象にする
					if id != nil {
						if sid := r.SetIdentifier; sid != nil && *id == *sid {
							ep.Id = *r.SetIdentifier
						} else {
							break
						}
					}

					if r.Weight != nil {
						ep.Weight = r.Weight
					}

					// TODO: owner idの考慮
					endpoints[owner] = ep
				}
			} else {
				break
			}
		}
	}
	return endpoints, nil
}

func (p *Route53Provider) AllRecords(ctx context.Context, zoneName string) ([]types.ResourceRecordSet, error) {
	var result []types.ResourceRecordSet

	params := &route53.ListResourceRecordSetsInput{
		HostedZoneId: &p.hostedZoneId,
	}
	for isTrunc := true; isTrunc; {
		output, err := p.client.ListResourceRecordSets(ctx, params)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to list resource records sets for zone %s", p.hostedZoneId)
		}
		result = append(result, output.ResourceRecordSets...)
		isTrunc = output.IsTruncated
		params.StartRecordName, params.StartRecordType, params.StartRecordIdentifier =
			output.NextRecordName, output.NextRecordType, output.NextRecordIdentifier
	}

	return result, nil
}

func buildFQDN(owner, zone string) string {
	fqdn := fmt.Sprintf("%s.%s", owner, zone)
	if !strings.HasSuffix(fqdn, ".") {
		fqdn = fqdn + "."
	}
	return fqdn
}

func isOwnerOfRecord(rrs types.ResourceRecordSet, ep endpoint.Endpoint, ownerId string) bool {
	// validate
	switch {
	case *rrs.Name != ep.DnsName:
	case rrs.Type != types.RRTypeTxt:
	case !strings.HasPrefix(*rrs.ResourceRecords[0].Value, recordOwnerPrefix):
	default:
		return true
	}
	return false
}

func buildOwnerRecordValue(ep endpoint.Endpoint, ownerId string) string {
	value := ownerId
	if ep.Id != "" {
		value = value + "-" + ep.Id
	}
	return value
}

func credFromSecretRef(ctx context.Context, p *dnsv1alpha1.Provider, c client.Client) (credentials.StaticCredentialsProvider, error) {
	secRef := p.Spec.Route53.Auth.SecretRef

	// get access key id from secret
	var ns string
	if secRef.AccessKeyID.Namespace != nil {
		ns = *secRef.AccessKeyID.Namespace
	} else {
		ns = p.Namespace
	}
	ke := client.ObjectKey{
		Name:      secRef.AccessKeyID.Name,
		Namespace: ns,
	}
	akSecret := v1.Secret{}
	err := c.Get(ctx, ke, &akSecret)
	if err != nil {
		return credentials.StaticCredentialsProvider{}, fmt.Errorf("failed to get access key id: %w", err)
	}

	// get secret access key from secret
	if secRef.SecretAccessKey.Namespace != nil {
		ns = *secRef.SecretAccessKey.Namespace
	} else {
		ns = p.Namespace
	}
	ke = client.ObjectKey{
		Name:      secRef.SecretAccessKey.Name,
		Namespace: ns,
	}
	sakSecret := v1.Secret{}
	err = c.Get(ctx, ke, &sakSecret)
	if err != nil {
		return credentials.StaticCredentialsProvider{}, fmt.Errorf("failed to get secret access key: %w", err)
	}

	ak := string(akSecret.Data[secRef.AccessKeyID.Key])
	sak := string(sakSecret.Data[secRef.SecretAccessKey.Key])
	if ak == "" {
		return credentials.StaticCredentialsProvider{}, fmt.Errorf("missing access key id")
	}
	if sak == "" {
		return credentials.StaticCredentialsProvider{}, fmt.Errorf("missing secret access key")
	}
	return credentials.NewStaticCredentialsProvider(ak, sak, ""), nil
}
