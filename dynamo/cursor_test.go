package dynamo

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/stretchr/testify/require"
)

type dynamoTestItem struct {
	PK    string
	SK    string
	Name  string
	Time  time.Time
	Count int
}

func TestCursorEncodeAndDecode(t *testing.T) {
	item := dynamoTestItem{
		PK:    "abc",
		SK:    "def",
		Name:  "Hello World",
		Time:  time.Now(),
		Count: 152,
	}

	key, err := attributevalue.MarshalMap(item)
	require.NoError(t, err)

	cursor, err := lastEvalKeyToCursor(key)
	require.NoError(t, err)

	keyBack, err := cursorToLastEval(cursor)
	require.NoError(t, err)

	require.Equal(t, key, keyBack)
}
