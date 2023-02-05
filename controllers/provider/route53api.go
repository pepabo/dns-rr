package provider

//go:generate mockgen -package provider -source=./route53api.go -destination=./route53api_mock.go

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/route53"
)

type Route53API interface {
	ListResourceRecordSets(ctx context.Context, params *route53.ListResourceRecordSetsInput, optFns ...func(*route53.Options)) (*route53.ListResourceRecordSetsOutput, error)
	ChangeResourceRecordSets(ctx context.Context, params *route53.ChangeResourceRecordSetsInput, optFns ...func(*route53.Options)) (*route53.ChangeResourceRecordSetsOutput, error)
}
