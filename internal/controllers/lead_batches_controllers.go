package controllers



import (
	//"os"
	//"io"
	//"fmt"
	//"log"
	//"time"
	"net/http"
	//"strconv"
	//"strings"
	//"encoding/csv"
	"database/sql"
	"github.com/google/uuid"
	//"golang.org/x/crypto/bcrypt"
	//"github.com/joho/godotenv"
	"github.com/gin-gonic/gin"
	//"github.com/DylanCoon99/lead_delivery_app/backend/internal/auth"
	"github.com/DylanCoon99/lead_delivery_app/backend/internal/utils"
	"github.com/DylanCoon99/lead_delivery_app/backend/internal/database/queries"
)



func (cfg *ApiConfig) ListLeadBatches(c *gin.Context) {

	/*
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

	buyers, err := cfg.DBQueries.ListBuyers(c, queries.ListBuyersParams{
		TenantID: tenantId,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list buyers"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"buyers": buyers})
	*/
}





func (cfg *ApiConfig) GetBatchInfo(c *gin.Context) {

	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}


	// Extract lead batch ID from URL parameter
	batchIDParam := c.Param("id")
	batchId, err := uuid.Parse(batchIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid lead batch ID"})
		return
	}

	batch, err := cfg.DBQueries.GetLeadBatchByID(c, queries.GetLeadBatchByIDParams{
		ID:       batchId,
		TenantID: tenantId,
	})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "lead batch not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"lead_batch": batch,
	})
}


func (cfg *ApiConfig) CreateBatch(c *gin.Context) {

	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}

	// Extract lead batch ID from URL parameter
	campaignIDParam := c.Param("campaign_id")
	campaignId, err := uuid.Parse(campaignIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid campaign id"})
		return
	}

	
	// get the user id from the token
	userId, err := utils.ExtractTokenUserID(header);
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get user id"})
		return
	}

	// get supplier id from the supplier_users table
	supplierId, err := cfg.DBQueries.GetSupplierForUser(c, userId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get user id"})
		return
	}


	var req struct {
		BatchName  string `json:"batch_name"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	totalLeads := sql.NullInt32{
		Int32: 0,
		Valid: true,
	}


	supplierIdNull := uuid.NullUUID{
		UUID: supplierId,
		Valid: true,
	} 


	lead_batch, err := cfg.DBQueries.CreateLeadBatch(c, queries.CreateLeadBatchParams{
		TenantID:   tenantId,
		CampaignID: campaignId,
		SupplierID: supplierIdNull,
		BatchName:  req.BatchName,
		TotalLeads: totalLeads,
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create lead_batch", "message": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"lead_batch": lead_batch})
}



func (cfg *ApiConfig) UpdateLeadBatchStatus(c *gin.Context) {

	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}

	// Extract lead batch ID from URL parameter
	batchIDParam := c.Param("id")
	batchId, err := uuid.Parse(batchIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid lead batch ID"})
		return
	}


	// Bind the request body
	var req struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}


	// Execute query
	updatedBatch, err := cfg.DBQueries.UpdateLeadBatchStatus(c, queries.UpdateLeadBatchStatusParams{
		ID:       batchId,
		TenantID: tenantId,
		Status:   sql.NullString{String: req.Status, Valid: true},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update lead batch status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":      "lead batch status updated successfully",
		"lead_batch":   updatedBatch,
	})
}



func (cfg *ApiConfig) DeleteBatch(c *gin.Context) {

	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}

	// Extract lead batch ID from URL parameter
	batchIDParam := c.Param("id")
	batchId, err := uuid.Parse(batchIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid lead batch ID"})
		return
	}

	// Execute delete query
	err = cfg.DBQueries.DeleteLeadBatch(c, queries.DeleteLeadBatchParams{
		ID:       batchId,
		TenantID: tenantId,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete lead batch"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "lead batch deleted successfully",
		"id":      batchId,
	})
}

func (cfg *ApiConfig) ListLeadBatchesForSupplier(c *gin.Context) {

	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}

	campaignIDStr := c.Query("campaign_id")
	if campaignIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "campaign_id query parameter is required",
			"message": "Please include ?campaign_id=<uuid> in your request",
		})
		return
	}
	campaignId, err := uuid.Parse(campaignIDStr)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid campaign ID"})
		return
	}

	// get the user id from the token
	userId, err := utils.ExtractTokenUserID(header);
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get user id"})
		return
	}

	// get supplier id from the supplier_users table
	supplierId, err := cfg.DBQueries.GetSupplierForUser(c, userId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get user id"})
		return
	}

	supplierIdNull := uuid.NullUUID{
		UUID: supplierId,
		Valid: true,
	} 


	// Query the database
	batches, err := cfg.DBQueries.ListLeadBatchesBySupplier(c, queries.ListLeadBatchesBySupplierParams{
		SupplierID: supplierIdNull,
		TenantID:   tenantId,
		CampaignID: campaignId,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch lead batches"})
		return
	}

	// Handle no results
	if len(batches) == 0 {
		c.JSON(http.StatusOK, gin.H{"lead_batches": []queries.LeadBatch{}})
		return
	}

	// Return lead batches
	c.JSON(http.StatusOK, gin.H{"lead_batches": batches})


}



func (cfg *ApiConfig) ListLeadBatchesForCampaign(c *gin.Context) { 


	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}


	campaignIDParam := c.Param("campaign_id")
	campaignId, err := uuid.Parse(campaignIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid campaign ID"})
		return
	}


	// Query the database
	batches, err := cfg.DBQueries.ListLeadBatchesByCampaign(c, queries.ListLeadBatchesByCampaignParams{
		CampaignID: campaignId,
		TenantID:   tenantId,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch lead batches"})
		return
	}

	// Handle no results
	if len(batches) == 0 {
		c.JSON(http.StatusOK, gin.H{"lead_batches": []queries.LeadBatch{}})
		return
	}

	// Return lead batches
	c.JSON(http.StatusOK, gin.H{"lead_batches": batches})



}