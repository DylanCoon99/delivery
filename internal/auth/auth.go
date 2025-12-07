package auth



import (
	"os"
	"time"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"github.com/golang-jwt/jwt/v5"
	//"github.com/joho/godotenv"
	"errors"
	"strings"
	//"net/http"
	"fmt"
	//"log"
	//"crypto/rand"
	//"encoding/hex"
)





func HashPassword(password string) (string, error) {

	hashed_password, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)

	if err != nil {
		return "", err
	}

	return string(hashed_password), nil
}




func CheckPassword(password, hash string) error {


	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))


	return err
}


type JWTClaims struct {
	UserID   uuid.UUID `json:"user_id"`
	TenantID uuid.UUID `json:"tenant_id"`
	Email    string    `json:"email"`
	Role     string    `json:"role"`
	jwt.RegisteredClaims
}


func (c *JWTClaims) Valid() error {
	if c.ExpiresAt != nil && !c.ExpiresAt.After(time.Now()) {
		return jwt.ErrTokenExpired
	}
	return nil
}


// GenerateJWT generates a JWT token with user ID, tenant ID, and role
func GenerateJWT(userID, tenantID uuid.UUID, role, email, tokenSecret string, expiresIn time.Duration) (string, error) {
	claims := JWTClaims{
		UserID:   userID,
		TenantID: tenantID,
		Email: email,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "lead_delivery_app",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiresIn)),
			Subject:   userID.String(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(tokenSecret))
}



func ValidateJWT(authHeader string) (uuid.UUID, error) {
	
	/*
	err := godotenv.Load(".env")

	if err != nil {
		log.Fatalf("Error loading .env file")
	}
	*/
	

	tokenSecret := os.Getenv("API_SECRET")


	if authHeader == "" {
		return uuid.Nil, errors.New("missing authorization header")
	}

	// Expect "Bearer <token>"
	tokenString := strings.TrimPrefix(authHeader, "Bearer ")
	if tokenString == authHeader {
		return uuid.Nil, errors.New("invalid token format (missing Bearer prefix)")
	}



	claimsStruct := jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(
		tokenString,
		&claimsStruct,
		func(token *jwt.Token) (interface{}, error) { return []byte(tokenSecret), nil },
	)
	if err != nil {
		return uuid.Nil, err
	}

	userIDString, err := token.Claims.GetSubject()
	if err != nil {
		return uuid.Nil, err
	}

	issuer, err := token.Claims.GetIssuer()
	if err != nil {
		return uuid.Nil, err
	}
	if issuer != string("lead_delivery_app") {
		return uuid.Nil, errors.New("invalid issuer")
	}

	id, err := uuid.Parse(userIDString)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid user ID: %w", err)
	}
	return id, nil
}



// Invite User JWT shit

type InviteClaims struct {
	UserID   string `json:"user_id"`
	TenantID string `json:"tenant_id"`
	Invite   bool   `json:"invite"`
	jwt.RegisteredClaims
}

func GenerateInviteToken(userID, tenantID string, duration time.Duration) (string, error) {

	tokenSecret := os.Getenv("API_SECRET")

	claims := InviteClaims{
		UserID:   userID,
		TenantID: tenantID,
		Invite:   true,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(duration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(tokenSecret)
}


