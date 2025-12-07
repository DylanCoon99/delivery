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




func (cfg *ApiConfig) ListLeadsForCampaign(c *gin.Context) {

	// Gets the leads a supplier has uploaded for a campaign

	// should be able to filter by campaign, supplier, and limit


	/*

	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}

	*/

}


func (cfg *ApiConfig) ListLeadsAdmin(c *gin.Context) {

	// should be able to filter by campaign, supplier, and limit


	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}

	

	campaignIDStr := c.Query("campaign_id")
	supplierIDStr := c.Query("supplier_id")
	limitStr := c.Query("limit")

	var campaignId uuid.UUID
	var supplierId uuid.UUID

	if campaignIDStr != "" {
		id, err := uuid.Parse(campaignIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid campaign_id"})
			return
		}
		campaignId = id
	}

	if supplierIDStr != "" {
		id, err := uuid.Parse(supplierIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid supplier_id"})
			return
		}
		supplierId = id
	}

	limit := int32(100)
	if limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = int32(parsed)
		}
	}

	leads, err := cfg.DBQueries.ListLeadsWithFilters(c, queries.ListLeadsWithFiltersParams{
		TenantID: tenantId,
		Column2:  campaignId,
		Column3:  supplierId,
		Limit:    limit,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch leads"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"count": len(leads),
		"leads": leads,
	})

}



func (cfg *ApiConfig) GetLeadByID(c *gin.Context) {

	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}


	idParam := c.Param("id")
	leadId, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid lead id"})
		return
	}

	lead, err := cfg.DBQueries.GetLeadByID(c, queries.GetLeadByIDParams{
		ID:       leadId,
		TenantID: tenantId,
	})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "lead not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"lead": lead})
}




func (cfg *ApiConfig) UpdateLead(c *gin.Context) {

	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}


	idParam := c.Param("id")
	campaignId, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid lead id"})
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


	err = cfg.DBQueries.UpdateCampaignStatus(c, queries.UpdateCampaignStatusParams{
		ID:       campaignId,
		TenantID: tenantId,
		IsActive: active,
	})
	
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "failed to update campaign"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "successfully updated the campaign"})
}



func (cfg *ApiConfig) DeleteLead(c *gin.Context) {

	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}

	idParam := c.Param("id")
	leadId, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid lead id"})
		return
	}

	err = cfg.DBQueries.DeleteLead(c, queries.DeleteLeadParams{
		ID:       leadId,
		TenantID: tenantId, 
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete lead"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "lead deleted"})

}


func (cfg *ApiConfig) ListLeadsForBatch(c *gin.Context) {

	header := c.GetHeader("Authorization")

	tenantId, err := utils.ExtractTokenTenantID(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
		return
	}


	leadBatchIdParam := c.Param("lead_batch_id")
	leadBatchId, err := uuid.Parse(leadBatchIdParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid lead batch id"})
		return
	}

	leadBatchIdNull := uuid.NullUUID{
		UUID: leadBatchId,
		Valid: true,
	}

	limit := int32(1000)
	offset := int32(0)

	leads, err := cfg.DBQueries.ListLeadsForBatch(c, queries.ListLeadsForBatchParams{
		TenantID:     tenantId,
		LeadBatchID:  leadBatchIdNull,
		Limit:        limit,
		Offset:       offset, 
	})

	// need to decrypt the PII for each lead
	for i := range leads {
		if leads[i].EmailHash.Valid {
			decryptedEmail, err := utils.DecryptString(leads[i].EmailHash.String)
			if err == nil {
				leads[i].EmailHash.String = decryptedEmail
			}
		}
		if leads[i].PhoneHash.Valid {
			decryptedPhone, err := utils.DecryptString(leads[i].PhoneHash.String)
			if err == nil {
				leads[i].PhoneHash.String = decryptedPhone
			}
		}
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get leads for batch"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"leads": leads})

}
