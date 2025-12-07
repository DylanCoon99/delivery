package utils


import (
	//"log"
	//"fmt"
	"os"
	//"strconv"
	//"strings"
	//"time"
	"context"
	"encoding/json"
	//"errors"
	"database/sql"
	//"golang.org/x/crypto/sha3"
	//"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	//jwt "github.com/dgrijalva/jwt-go"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)


type DBSecret struct {
	Username string `json:"username"`
	Password string `json:"password"`
}


func GetDBSecret() (*DBSecret, error) {
	secretName := os.Getenv("DB_SECRET_NAME")
	region := os.Getenv("AWS_REGION")
	
	if secretName == "" {
		secretName = "rds!cluster-61b64ebc-5f07-4e23-b316-94361855308a" // fallback
	}
	if region == "" {
		region = "us-east-2" // fallback
	}

	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
	if err != nil {
		return nil, err
	}

	svc := secretsmanager.NewFromConfig(cfg)
	input := &secretsmanager.GetSecretValueInput{
		SecretId:     aws.String(secretName),
		VersionStage: aws.String("AWSCURRENT"),
	}

	result, err := svc.GetSecretValue(context.TODO(), input)
	if err != nil {
		return nil, err
	}

	var secret DBSecret
	err = json.Unmarshal([]byte(*result.SecretString), &secret)
	if err != nil {
		return nil, err
	}

	return &secret, nil
}



// Utility — safely wraps string into sql.NullString
func SqlNullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}


func SafeString(s *string) string {
    if s == nil {
        return ""
    }
    return *s
}

func SafeBool(b *bool) bool {
    if b == nil {
        return false
    }
    return *b
}

func NullUUID(id uuid.UUID) uuid.NullUUID {

	return uuid.NullUUID{
		UUID: id,
		Valid: true,
	}

}

