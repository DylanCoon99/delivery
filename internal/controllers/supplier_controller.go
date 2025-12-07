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
	"github.com/DylanCoon99/lead_delivery_app/backend/internal/utils"
	"github.com/DylanCoon99/lead_delivery_app/backend/internal/database/queries"
)



func (cfg *ApiConfig) ListSuppliers(c *gin.Context) {

	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}


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

	suppliers, err := cfg.DBQueries.ListSuppliers(c, queries.ListSuppliersParams{
		TenantID: tenantId,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list suppliers"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"suppliers": suppliers})
}



func (cfg *ApiConfig) GetSupplierByID(c *gin.Context) {

	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}


	idParam := c.Param("id")
	supplierId, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid supplier id"})
		return
	}

	supplier, err := cfg.DBQueries.GetSupplierByID(c, queries.GetSupplierByIDParams{
		ID:       supplierId,
		TenantID: tenantId,
	})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "supplier not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"supplier": supplier})
}


func (cfg *ApiConfig) CreateSupplier(c *gin.Context) {

	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}

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


	supplier, err := cfg.DBQueries.CreateSupplier(c, queries.CreateSupplierParams{
		TenantID:     tenantId,
		Name:         req.Name,
		ContactEmail: email,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create supplier"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"supplier": supplier})
}



func (cfg *ApiConfig) UpdateSupplier(c *gin.Context) {

	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}


	idParam := c.Param("id")
	supplierId, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid supplier id"})
		return
	}

	var req struct {
		Name         string    `json:"name"`
		ContactEmail string    `json:"contact_email"`
		APIKey       string    `json:"api_key"`
		IsActive     bool      `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	email := sql.NullString{
		String: req.ContactEmail,
		Valid:  req.ContactEmail != "",
	}

	apiKey := sql.NullString{
		String: req.APIKey,
		Valid:  req.APIKey != "",
	}

	is_active := sql.NullBool{
		Bool:   req.IsActive,
		Valid:  true,
	}

	supplier, err := cfg.DBQueries.UpdateSupplier(c, queries.UpdateSupplierParams{
		ID:           supplierId,
		TenantID:     tenantId,
		Name:         req.Name,
		ContactEmail: email,
		ApiKey:       apiKey,
		IsActive:     is_active,
	})
	
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "failed to update supplier"})
		return
	}


	err = cfg.DBQueries.UpdateUserStatusBySupplier(c, queries.UpdateUserStatusBySupplierParams{
		SupplierID: supplierId,
		IsActive:   sql.NullBool{
			Bool:  req.IsActive,
			Valid: true,
		},
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update supplier users"})
		return
	}


	c.JSON(http.StatusOK, gin.H{"supplier": supplier})
}



func (cfg *ApiConfig) DeleteSupplier(c *gin.Context) {

	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}


	idParam := c.Param("id")
	supplierId, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid supplier id"})
		return
	}

	err = cfg.DBQueries.DeleteSupplier(c, queries.DeleteSupplierParams{
		ID:       supplierId,
		TenantID: tenantId,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete supplier"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "supplier deleted"})
}





/*
func (cfg *ApiConfig) UpdateUsersBySupplierStatus(c *gin.Context) {
    idParam := c.Param("id")
    supplierId, err := uuid.Parse(idParam)

    if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid supplier id"})
		return
	}

    var req struct {
    	IsActive bool `json:"is_active"`
    }


    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "message": err.Error()})
        return
    }

    // UpdateUserStatusBySupplierParams

    err = cfg.DBQueries.UpdateUserStatusBySupplier(c, queries.UpdateUserStatusBySupplierParams{
		SupplierID: supplierId,
		IsActive:   sql.NullBool{
			Bool:  req.IsActive,
			Valid: true,
		},
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update supplier users"})
		return
	}

    c.JSON(http.StatusOK, gin.H{"message": "User statuses updated successfully"})
}
*/
