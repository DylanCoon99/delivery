package controllers


import (
    //"os"
    //"log"
    //"time"
    //"errors"
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



func (cfg *ApiConfig) GetDeliveryJob(c *gin.Context) {

    header := c.GetHeader("Authorization")

    tenantId, err := utils.ExtractTokenTenantID(header)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
        return
    }

    // --- Job ID from URL ---
    idParam := c.Param("id")
    jobID, err := uuid.Parse(idParam)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid delivery job id"})
        return
    }

    // --- Fetch delivery job ---
    job, err := cfg.DBQueries.GetDeliveryJob(c, queries.GetDeliveryJobParams{
        ID:       jobID,
        TenantID: tenantId,
    })
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "error":   "failed to fetch delivery job",
            "message": err.Error(),
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "tenant_id":     tenantId,
        "delivery_job":  job,
    })
}


func (cfg *ApiConfig) UpdateDeliveryJob(c *gin.Context) {

    header := c.GetHeader("Authorization")

    tenantId, err := utils.ExtractTokenTenantID(header)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
        return
    }

    // --- Job ID from URL ---
    idParam := c.Param("id")
    jobID, err := uuid.Parse(idParam)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid delivery job id"})
        return
    }

    // --- Request Body ---
    var req struct {
        Status    string `json:"status"`
        LastError string `json:"last_error"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }



    lastErr := sql.NullString{
        String: req.LastError,
        Valid:  req.LastError != "",
    }

    // --- Update job ---
    job, err := cfg.DBQueries.UpdateDeliveryJobStatus(c, queries.UpdateDeliveryJobStatusParams{
        ID:        jobID,
        Status:    req.Status,
        LastError: lastErr,
        TenantID:  tenantId,
    })
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "error":   "failed to update delivery job",
            "message": err.Error(),
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "message":       "successfully updated delivery job",
        "delivery_job":  job,
        "tenant_id":     tenantId,
    })
}


func (cfg *ApiConfig) DeleteDeliveryJob(c *gin.Context) {
    header := c.GetHeader("Authorization")

    tenantId, err := utils.ExtractTokenTenantID(header)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
        return
    }

    // --- Job ID from URL ---
    idParam := c.Param("id")
    jobID, err := uuid.Parse(idParam)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid delivery job id"})
        return
    }

    // --- Execute delete ---
    err = cfg.DBQueries.DeleteDeliveryJob(c, queries.DeleteDeliveryJobParams{
        ID:       jobID,
        TenantID: tenantId,
    })
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "error":   "failed to delete delivery job",
            "message": err.Error(),
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "message":     "delivery job deleted",
        "job_id":      jobID,
        "tenant_id":   tenantId,
    })
}


func (cfg *ApiConfig) ListPendingDeliveryJobs(c *gin.Context) {
    header := c.GetHeader("Authorization")

    tenantId, err := utils.ExtractTokenTenantID(header)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
        return
    }

    // --- Optional query param for limit ---
    limitParam := c.Query("limit")
    var limit int32 = 100 // default
    if limitParam != "" {
        parsedLimit, err := strconv.ParseInt(limitParam, 10, 32)
        if err == nil && parsedLimit > 0 {
            limit = int32(parsedLimit)
        }
    }

    // --- Fetch pending jobs ---
    jobs, err := cfg.DBQueries.ListPendingJobs(c, queries.ListPendingJobsParams{
        Limit:    limit,
        TenantID: tenantId,
    })
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "error":   "failed to fetch pending delivery jobs",
            "message": err.Error(),
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "tenant_id":       tenantId,
        "pending_jobs":    jobs,
        "count":           len(jobs),
    })
}

