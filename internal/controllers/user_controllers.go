package controllers


import (
	//"os"
	//"log"
	//"time"
	"net/http"
	//"encoding/json"
	"database/sql"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	//"github.com/joho/godotenv"
	"github.com/gin-gonic/gin"
	"github.com/DylanCoon99/lead_delivery_app/backend/internal/auth"
	"github.com/DylanCoon99/lead_delivery_app/backend/internal/utils"
	"github.com/DylanCoon99/lead_delivery_app/backend/internal/database/queries"
)




func (cfg *ApiConfig) GetCurrentUser(c *gin.Context) {
	claims, exists := c.Get("claims")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing token claims"})
		return
	}
	userClaims := claims.(*auth.JWTClaims)

	user, err := cfg.DBQueries.GetUserByID(c, queries.GetUserByIDParams{
		ID:       userClaims.UserID,
		TenantID: userClaims.TenantID,
	})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"user": user})
}



func (cfg *ApiConfig) ListUsers(c *gin.Context) {
	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}


	users, err := cfg.DBQueries.ListUsers(c, tenantId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list users"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"users": users})
}




func (cfg *ApiConfig) UpdateUserPassword(c *gin.Context) {
	claims, exists := c.Get("claims")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing token claims"})
		return
	}
	userClaims := claims.(*auth.JWTClaims)

	var req struct {
		UserID   uuid.UUID `json:"user_id" binding:"required"`
		Password string    `json:"password" binding:"required,min=8"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}

	user, err := cfg.DBQueries.UpdateUserPassword(c, queries.UpdateUserPasswordParams{
		ID:           req.UserID,
		TenantID:     userClaims.TenantID,
		PasswordHash: string(hashed),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update password"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"user": user})
}



func (cfg *ApiConfig) DeleteUser(c *gin.Context) {
	claims, exists := c.Get("claims")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing token claims"})
		return
	}
	userClaims := claims.(*auth.JWTClaims)

	userIDParam := c.Param("id")
	userID, err := uuid.Parse(userIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	err = cfg.DBQueries.DeleteUser(c, queries.DeleteUserParams{
		ID:       userID,
		TenantID: userClaims.TenantID,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "user deleted"})
}



func (cfg *ApiConfig) GetUser(c *gin.Context) {
	claims, exists := c.Get("claims")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing token claims"})
		return
	}
	userClaims := claims.(*auth.JWTClaims)

	userIDParam := c.Param("id")
	userID, err := uuid.Parse(userIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	user, err := cfg.DBQueries.GetUserByID(c, queries.GetUserByIDParams{
		ID:       userID,
		TenantID: userClaims.TenantID,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"user": user})
}


func (cfg *ApiConfig) UpdateUserStatus(c *gin.Context) {


	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}


	idParam := c.Param("id")
	userId, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	var req struct {
		IsActive   bool    `json:"is_active"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	active := sql.NullBool{Bool: req.IsActive, Valid: true}


	err = cfg.DBQueries.UpdateUserStatus(c, queries.UpdateUserStatusParams{
		ID:       userId,
		TenantID: tenantId,
		IsActive: active,
	})
	
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "failed to update user status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "successfully updated the user status"})
}



func (cfg *ApiConfig) UpdateUserRole(c *gin.Context) {


	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}


	idParam := c.Param("id")
	userId, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	var req struct {
		Role   string    `json:"role"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}


	_, err = cfg.DBQueries.UpdateUserRole(c, queries.UpdateUserRoleParams{
		ID:       userId,
		TenantID: tenantId,
		Role:     req.Role,
	})
	
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "failed to update user role"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "successfully updated the user role"})
}


func (cfg *ApiConfig) GetTenantAdminEmail(c *gin.Context) {

	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}


	emailNullStr, err := cfg.DBQueries.GetTenantAdminEmail(c, tenantId)
	
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "failed to get contact email"})
		return
	}

	email := ""

	if emailNullStr.Valid {
		email = emailNullStr.String
	}

	c.JSON(http.StatusOK, gin.H{"contact_email": email})	


}

