package utils


import (
	//"log"
	//"fmt"
	"os"
	//"strconv"
	"strings"
	//"time"
	"context"
	"encoding/json"
	"errors"
	"database/sql"
	//"golang.org/x/crypto/sha3"
	//"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/DylanCoon99/lead_delivery_app/backend/internal/auth"
	jwt "github.com/dgrijalva/jwt-go"
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


func ExtractTokenRole(authHeader string) (string, error) {
	if authHeader == "" {
		return "", errors.New("missing authorization header")
	}

	// Expect "Bearer <token>"
	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
	if tokenStr == authHeader {
		return "", errors.New("invalid token format (missing Bearer prefix)")
	}

	// Parse the token
	token, err := jwt.ParseWithClaims(
		tokenStr,
		&auth.JWTClaims{},
		func(token *jwt.Token) (interface{}, error) {
			return []byte(getJWTSecret()), nil // helper to load API_SECRET
		},
	)
	if err != nil {
		return "", err
	}

	claims, ok := token.Claims.(*auth.JWTClaims)
	if !ok || !token.Valid {
		return "", errors.New("invalid token claims")
	}

	return claims.Role, nil
}


func ExtractTokenTenantID(authHeader string) (uuid.UUID, error) {
	if authHeader == "" {
		return uuid.Nil, errors.New("missing authorization header")
	}

	// Expect "Bearer <token>"
	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
	if tokenStr == authHeader {
		return uuid.Nil, errors.New("invalid token format (missing Bearer prefix)")
	}

    token, _, err := new(jwt.Parser).ParseUnverified(tokenStr, &auth.JWTClaims{})
    if err != nil {
        return uuid.Nil, errors.New("invalid token format")
    }

    claims, ok := token.Claims.(*auth.JWTClaims)
    if !ok {
        return uuid.Nil, errors.New("invalid token claims")
    }

    if claims.TenantID == uuid.Nil {
        return uuid.Nil, errors.New("tenant id not found in token")
    }

    return claims.TenantID, nil
}



func ExtractTokenUserID(authHeader string) (uuid.UUID, error) {
	if authHeader == "" {
		return uuid.Nil, errors.New("missing authorization header")
	}

	// Expect "Bearer <token>"
	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
	if tokenStr == authHeader {
		return uuid.Nil, errors.New("invalid token format (missing Bearer prefix)")
	}

    token, _, err := new(jwt.Parser).ParseUnverified(tokenStr, &auth.JWTClaims{})
    if err != nil {
        return uuid.Nil, errors.New("invalid token format")
    }

    claims, ok := token.Claims.(*auth.JWTClaims)
    if !ok {
        return uuid.Nil, errors.New("invalid token claims")
    }

    if claims.UserID == uuid.Nil {
        return uuid.Nil, errors.New("user id not found in token")
    }

    return claims.UserID, nil
}


func getJWTSecret() string {
	// In production, load from env only once or pass via config
	secret := os.Getenv("API_SECRET")
	if secret == "" {
		panic("missing API_SECRET environment variable")
	}
	return secret
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

