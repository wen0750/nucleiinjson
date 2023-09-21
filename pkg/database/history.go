package database

import (
	"context"
	"time"

	"log"

	"github.com/wen0750/nucleiinjson/pkg/output"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

var connection *mongo.Collection

func init() {
	connection, err := InitializeMongoDB("History")
	if err != nil {
		log.Fatalf("Error initializing MongoDB Folders collection: %v\n", err)
	} else {
		collection = connection
		log.Println("MongoDB (folder) initialized successfully")
	}
}

func InsertHistoryRecord(data *output.ResultEvent, fid string) (bson.M, error) {
	var result bson.M

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	objID, _ := primitive.ObjectIDFromHex(fid)
	filter := bson.M{"_id": objID}
	update := bson.M{
		"$push": bson.M{"result": data},
	}

	err := connection.FindOneAndUpdate(ctx, filter, update).Decode(&result)
	return result, err
}
