package controllers


import (
	//"os"
	//"log"
	//"time"
	"net/http"
	"strconv"
	//"encoding/json"
	"database/sql"
	"github.com/google/uuid"
	//"golang.org/x/crypto/bcrypt"
	//"github.com/joho/godotenv"
	"github.com/gin-gonic/gin"
	//"github.com/DylanCoon99/lead_delivery_app/backend/internal/auth"
	"github.com/DylanCoon99/lead_delivery_app/backend/internal/database/queries"
)



func (cfg *ApiConfig) ListTenants(c *gin.Context) {


	// Optional query params for pagination
	limit := int32(50) // default
	offset := int32(0) // default
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil {
			limit = int32(parsed)
		}
	}
	if o := c.Query("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil {
			offset = int32(parsed)
		}
	}

	tenants, err := cfg.DBQueries.ListTenants(c, queries.ListTenantsParams{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list tenants"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"tenants": tenants})
}



func (cfg *ApiConfig) GetTenantByID(c *gin.Context) {


	idParam := c.Param("id")
	tenantID, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tenant id"})
		return
	}

	tenant, err := cfg.DBQueries.GetTenantByID(c, tenantID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tenant not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"tenant": tenant})
}


func (cfg *ApiConfig) CreateTenant(c *gin.Context) {


	var req struct {
		Name         string `json:"name" binding:"required"`
		ContactEmail string `json:"contact_email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	email := sql.NullString{
		String: req.ContactEmail,
		Valid:  req.ContactEmail != "",
	}


	tenant, err := cfg.DBQueries.CreateTenant(c, queries.CreateTenantParams{
		Name:         req.Name,
		ContactEmail: email,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create tenant"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"tenant": tenant})
}



func (cfg *ApiConfig) UpdateTenant(c *gin.Context) {


	idParam := c.Param("id")
	tenantID, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tenant id"})
		return
	}

	var req struct {
		Name         string `json:"name"`
		ContactEmail string `json:"contact_email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	email := sql.NullString{
		String: req.ContactEmail,
		Valid:  req.ContactEmail != "",
	}

	tenant, err := cfg.DBQueries.UpdateTenant(c, queries.UpdateTenantParams{
		ID:           tenantID,
		Name:         req.Name,
		ContactEmail: email,
	})
	
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update tenant"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"tenant": tenant})
}



func (cfg *ApiConfig) DeleteTenant(c *gin.Context) {


	idParam := c.Param("id")
	tenantID, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tenant id"})
		return
	}

	err = cfg.DBQueries.DeleteTenant(c, tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete tenant"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "tenant deleted"})
}
