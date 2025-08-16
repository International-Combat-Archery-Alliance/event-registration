package dynamo

import (
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

const (
	gsi1 = "GSI1"
)

type DB struct {
	dynamoClient *dynamodb.Client
	tableName    string
}

func NewDB(dynamoClient *dynamodb.Client, tableName string) *DB {
	return &DB{
		dynamoClient: dynamoClient,
		tableName:    tableName,
	}
}
