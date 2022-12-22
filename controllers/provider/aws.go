package provider

import (
	"context"
	"fmt"
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
)

const recordOwnerPrefix = "dns-rr-owner: "

type Route53Provider struct {
	hostedZoneId string
	client       *route53.Client
}

type endpoint struct {
        // The Dns Name
        dnsName string
        // The type of DNS Record
        class string

        // The value of DNS Record
        rdata string

        // The TTL of DNS Record
        ttl int64

        // The Id of DNS Record
        id string

        // The owner of DNS Record
        resourceOwner string
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
        var changes []types.Change
        desired := endpoint{
                class: rrSpec.Class,
                rdata: rrSpec.Rdata,
                ttl: int64(rrSpec.Ttl),
        }

        currentRecords, err := p.records(ctx, zoneId, zoneName, owners, rrSpec.Class)
        if err != nil {
                return err
        }

        for _, owner := range owners {
                fqdn := buildFQDN(owner, zoneName) 
                desired.dnsName = fqdn
                c := types.Change{
                        ResourceRecordSet: &types.ResourceRecordSet{
                                Name: aws.String(fqdn),
                                Type: types.RRType(desired.class),
                                ResourceRecords: []types.ResourceRecord{{Value: &desired.rdata}},
                                TTL: &desired.ttl,
                        },
                }
                if _, exist := currentRecords[owner]; !exist {
                        // レコードが存在しなかった場合
                        c.Action = types.ChangeActionCreate
                        changes = append(changes, c)
                } else if desired != currentRecords[owner] {
                        // 値が異なる場合
                        c.Action = types.ChangeActionUpsert
                        changes = append(changes, c)
                }
        }

        // 更新
        for i:=0; i<len(changes); i++ {
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

func (p *Route53Provider) records(ctx context.Context, zoneId string, zoneName string, owners []string, recordType string) (map[string]endpoint, error) {
        endpoints := make(map[string]endpoint, len(owners))
        for _, owner := range owners{
                fqdn := buildFQDN(owner, zoneName)
                // oenwer から始まるrecordsetをリスト
                params := &route53.ListResourceRecordSetsInput{
                        HostedZoneId: &zoneId,
                        StartRecordName: aws.String(fqdn),
                }
                output, err := p.client.ListResourceRecordSets(ctx, params)
                if err != nil {
                        return nil, errors.Wrapf(err, "failed to list resource records sets for zone %s", zoneId)
                }

                // 一致するレコードを検索
                ep := endpoint{
                        dnsName: fqdn,
                        class: recordType,
                }
                for _, r := range output.ResourceRecordSets {
                        if *r.Name == fqdn {
                                if r.Type == types.RRType(recordType) {
                                        // TODO: multi value レコードを考慮する
                                        ep.rdata = *r.ResourceRecords[0].Value
                                        ep.ttl = *r.TTL
                                        if r.SetIdentifier != nil {
                                                ep.id = *r.SetIdentifier
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

func buildFQDN(owner, zone string) string {
        fqdn := fmt.Sprintf("%s.%s", owner, zone)
	if !strings.HasSuffix(".", fqdn) {
		fqdn = fqdn + "."
	}
        return fqdn
}

func isOwnerOfRecord(rrs types.ResourceRecordSet, ep endpoint, ownerId string) bool {
        // validate
        switch {
        case *rrs.Name != ep.dnsName:
        case rrs.Type != types.RRTypeTxt:
        case !strings.HasPrefix(*rrs.ResourceRecords[0].Value, recordOwnerPrefix):
        default:
                return true
        }
        return false
}

func buildOwnerRecordValue(ep endpoint, ownerId string) string {
        value := ownerId
        if ep.id != "" {
                value = value + "-" + ep.id
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
