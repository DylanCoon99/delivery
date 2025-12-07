package controllers


import (
	"os"
	//"log"
	//"fmt"
	"errors"
	"net/http"
	//"strconv"
	//"encoding/json"
	//"database/sql"
	"github.com/lib/pq"
	"time"
	"github.com/google/uuid"
	//"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
	"github.com/gin-gonic/gin"
	"github.com/DylanCoon99/lead_delivery_app/backend/internal/auth"
	"github.com/DylanCoon99/lead_delivery_app/backend/internal/database/queries"
	//"github.com/DylanCoon99/lead_delivery_app/backend/internal/utils"
)

func (cfg *ApiConfig) RegisterUser(c *gin.Context) {
	var req struct {
		TenantID    uuid.UUID `json:"tenant_id" binding:"required"`
		Email       string    `json:"email" binding:"required,email"`
		Password    string    `json:"password"`
		Role        string    `json:"role" binding:"required,oneof=admin supplier buyer manager super_admin"`
		SupplierID  uuid.UUID `json:"supplier_id,omitempty"` // optional
	    BuyerID     uuid.UUID `json:"buyer_id,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// make sure the email is unique


	hashed, err := auth.HashPassword(req.Password)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}

	user, err := cfg.DBQueries.CreateUser(c, queries.CreateUserParams{
		TenantID:     req.TenantID,
		Email:        req.Email,
		PasswordHash: string(hashed),
		Role:         req.Role,
	})
	if err != nil {
        var pqErr *pq.Error
        if errors.As(err, &pqErr) {
            if pqErr.Code == "23505" { // unique_violation
                c.JSON(http.StatusConflict, gin.H{
                    "error": "A user with this email already exists for this tenant",
                })
                return
            }
        }

        c.JSON(http.StatusInternalServerError, gin.H{
            "error": "Failed to register user",
            "details": err.Error(),
        })
    }


	if req.Role == "supplier" {
	    _, err = cfg.DBQueries.CreateSupplierUser(c, queries.CreateSupplierUserParams{
	        TenantID:    req.TenantID,
	        SupplierID:  req.SupplierID,
	        UserID:      user.ID,
	    })
	    if err != nil {
	    	c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create supplier user relation", "message": err.Error()})
	    	return
	    }
	} else if req.Role == "buyer" {
	    _, err = cfg.DBQueries.CreateBuyerUser(c, queries.CreateBuyerUserParams{
	        TenantID:    req.TenantID,
	        BuyerID:     req.BuyerID,
	        UserID:      user.ID,
	    })
	    if err != nil {
	    	c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create buyer user relation", "message": err.Error()})
	    	return
	    }
	}

	c.JSON(http.StatusCreated, gin.H{"user": user})
}




func (cfg *ApiConfig) LoginUser(c *gin.Context) {
	var req struct {
		TenantID uuid.UUID `json:"tenant_id" binding:"required"`
		Email    string    `json:"email" binding:"required,email"`
		Password string    `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := cfg.DBQueries.GetUserByEmail(c, queries.GetUserByEmailParams{
		Email:    req.Email,
		TenantID: req.TenantID, 
	})
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Failed to get user from database.", "message": err.Error()})
		return
	}

	if !user.IsActive.Bool {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User is not active. Contact Admin"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	// JWT_SECRET
	
	/*
	err = godotenv.Load(".env")

	if err != nil {
		log.Fatalf("Error loading .env file")
	}
	*/

	

	jwtSecret := os.Getenv("API_SECRET")

	

	token, err := auth.GenerateJWT(user.ID, user.TenantID, user.Role, user.Email, jwtSecret, time.Hour*24)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": token})
}




func (cfg *ApiConfig) GetHashedPassword(c *gin.Context) {

	passwordParam := c.Param("password")

	hashed_password, err := bcrypt.GenerateFromPassword([]byte(passwordParam), bcrypt.DefaultCost)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
	}
	

	c.JSON(http.StatusOK, gin.H{"password": string(hashed_password)})

}


/*
func (cfg *ApiConfig) InviteUser(c *gin.Context) {
	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	header := c.GetHeader("Authorization")
	tenantID, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid tenant"})
		return
	}

	// Step 1: Find or create user (without password)
	user, err := cfg.DBQueries.GetUserByEmail(c, queries.GetUserByEmailParams{
		Email:    req.Email,
		TenantID: tenantID, 
	})
	if err != nil {
		// Create a new user
		user, err = cfg.DBQueries.CreateUser(c, queries.CreateUserParams{
			Email:     req.Email,
			Role:      req.Role,
			TenantID:  tenantID,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
			return
		}
	}

	// Step 2: Generate invite JWT (valid for 24h)
	token, err := auth.GenerateInviteToken(user.ID.String(), tenantID.String(), 24*time.Hour)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	// Step 3: Build invite link
	inviteURL := fmt.Sprintf("https://yourapp.com/set-password?token=%s", token)

	// Step 4: Send invite email
	err = cfg.EmailClient.SendInviteEmail(user.Email, inviteURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to send email"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "invite sent successfully",
		"invite_link": inviteURL, // optional, helpful for dev testing
	})
}

*/