package examples

import (
	"context"
	"testing"

	"github.com/gustavosbarreto/dbtest"
	_ "github.com/gustavosbarreto/dbtest/mongodb"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestMongoDB(t *testing.T) {
	server := dbtest.New("mongodb")
	defer server.Stop()

	client, ok := server.Client().(*mongo.Client)
	if !ok {
		t.Fatal("failed to get mongo client")
	}

	doc := bson.M{"name": "gustavo"}
	coll := client.Database("test").Collection("users")
	_, err := coll.InsertOne(context.TODO(), doc)
	assert.NoError(t, err)

	var result bson.M
	err = coll.FindOne(context.TODO(), bson.M{"name": "gustavo"}).Decode(&result)
	assert.NoError(t, err)
	assert.Equal(t, doc["name"], result["name"])
}
