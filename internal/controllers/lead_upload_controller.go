package controllers

import (
    //"os"
    "io"
    "fmt"
    //"log"
    //"time"
    "regexp"
    "net/http"
    //"net"
    //"strconv"
    "strings"
    "encoding/csv"
    //"database/sql"
    "github.com/google/uuid"
    //"github.com/sqlc-dev/pqtype"
    //"golang.org/x/crypto/bcrypt"
    //"github.com/joho/godotenv"
    "github.com/gin-gonic/gin"
    //"github.com/DylanCoon99/lead_delivery_app/backend/internal/auth"
    "github.com/DylanCoon99/lead_delivery_app/backend/internal/utils"
    "github.com/DylanCoon99/lead_delivery_app/backend/internal/database/queries"
)



type LeadInput struct {
    FullName            string
    Email               string
    Phone               string
    IPAddress           string
    CompanyName         string
    Address             string
    CountryCode         string
    LinkedInContact     string
    LinkedInCompany     string
    DownloadedAssetName string
    PublisherName       string
    Industry            string
    RevenueSize         string
    EmployeeSize        string
    Region              string
    State               string
    RowNum              int
}





func (cfg *ApiConfig) UploadLeadsToBatch(c *gin.Context) {
    header := c.GetHeader("Authorization")
    tenantId, err := utils.ExtractTokenTenantID(header)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant id"})
        return
    }

    batchIDParam := c.Param("id")
    batchId, err := uuid.Parse(batchIDParam)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid batch ID"})
        return
    }

    // get campaign id from the batch
    batch, err := cfg.DBQueries.GetLeadBatchByID(c, queries.GetLeadBatchByIDParams{
        ID:       batchId,
        TenantID: tenantId,
    })
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "lead batch not found"})
        return
    }
    campaignId := batch.CampaignID

    // fetch suppression list for this campaign
    suppressionListID, err := cfg.DBQueries.GetSuppressionListForCampaign(c, queries.GetSuppressionListForCampaignParams{
        ID:       campaignId,
        TenantID: tenantId,
    })
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get suppression list"})
        return
    }

    // get suppression entries (emails and phones)
    suppressionEntries, _ := cfg.DBQueries.GetSuppressionEntriesByListID(c, suppressionListID)

    // build sets for faster lookup
    emailSuppressionSet := make(map[string]bool)
    phoneSuppressionSet := make(map[string]bool)
    for _, e := range suppressionEntries {
        if e.EmailHash.Valid {
            emailSuppressionSet[e.EmailHash.String] = true
        }
        if e.PhoneHash.Valid {
            phoneSuppressionSet[e.PhoneHash.String] = true
        }
    }

    // fetch suppression attributes
    suppressedCompanies, _ := cfg.DBQueries.GetSuppressionCompaniesByListID(c, suppressionListID)
    suppressedRegions, _ := cfg.DBQueries.GetSuppressionRegionsByListID(c, suppressionListID)
    suppressedStates, _ := cfg.DBQueries.GetSuppressionStatesByListID(c, suppressionListID)
    suppressedEmployeeSizes, _ := cfg.DBQueries.GetSuppressionEmployeeSizesByListID(c, suppressionListID)
    suppressedRevenueSizes, _ := cfg.DBQueries.GetSuppressionRevenueSizesByListID(c, suppressionListID)

    // convert to maps for lookup
    companySet := make(map[string]bool)
    for _, v := range suppressedCompanies {
        companySet[strings.ToLower(v.CompanyName)] = true
    }
    regionSet := make(map[string]bool)
    for _, v := range suppressedRegions {
        regionSet[strings.ToLower(v.RegionName)] = true
    }
    stateSet := make(map[string]bool)
    for _, v := range suppressedStates {
        stateSet[strings.ToLower(v.StateName)] = true
    }
    empSizeSet := make(map[string]bool)
    for _, v := range suppressedEmployeeSizes {
        empSizeSet[strings.ToLower(v.SizeRange)] = true
    }
    revenueSet := make(map[string]bool)
    for _, v := range suppressedRevenueSizes {
        revenueSet[strings.ToLower(v.RevenueRange)] = true
    }

    // read uploaded CSV
    file, err := c.FormFile("file")
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "missing CSV file"})
        return
    }

    src, err := file.Open()
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to open file"})
        return
    }
    defer src.Close()

    reader := csv.NewReader(src)
    headerRead := false
    var leads []LeadInput

    for {
        record, err := reader.Read()
        if err == io.EOF {
            break
        }
        if err != nil {
            c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("error reading csv: %v", err)})
            return
        }

        if !headerRead {
            headerRead = true
            continue
        }

        if len(record) < 16 {
            c.JSON(http.StatusBadRequest, gin.H{"error": "invalid CSV format — expected 16 columns"})
            return
        }

        leads = append(leads, LeadInput{
            FullName:            strings.TrimSpace(record[0]),
            Email:               strings.TrimSpace(record[1]),
            Phone:               strings.TrimSpace(record[2]),
            IPAddress:           strings.TrimSpace(record[3]),
            CompanyName:         strings.TrimSpace(record[4]),
            Address:             strings.TrimSpace(record[5]),
            CountryCode:         strings.TrimSpace(record[6]),
            LinkedInContact:     strings.TrimSpace(record[7]),
            LinkedInCompany:     strings.TrimSpace(record[8]),
            DownloadedAssetName: strings.TrimSpace(record[9]),
            PublisherName:       strings.TrimSpace(record[10]),
            Industry:            strings.TrimSpace(record[11]),
            RevenueSize:         strings.TrimSpace(record[12]),
            EmployeeSize:        strings.TrimSpace(record[13]),
            Region:              strings.TrimSpace(record[14]),
            State:               strings.TrimSpace(record[15]),
            RowNum:              len(leads) + 1,
        })

    }

    // validate and apply suppression rules
    var validationErrors []string
    emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
    phoneRegex := regexp.MustCompile(`^[0-9\-\+\(\) ]{7,15}$`)

    for _, lead := range leads {
        row := lead.RowNum
        // 1️⃣ Base validation
        if lead.Email == "" && lead.Phone == "" {
            validationErrors = append(validationErrors, fmt.Sprintf("Row %d: Missing email or phone", row))
            continue
        }
        if lead.Email != "" && !emailRegex.MatchString(lead.Email) {
            validationErrors = append(validationErrors, fmt.Sprintf("Row %d: Invalid email '%s'", row, lead.Email))
            continue
        }
        if lead.Phone != "" && !phoneRegex.MatchString(lead.Phone) {
            validationErrors = append(validationErrors, fmt.Sprintf("Row %d: Invalid phone '%s'", row, lead.Phone))
            continue
        }

        // 2️⃣ Suppression checks
        emailHash, _ := utils.EncryptString(lead.Email)
        phoneHash, _ := utils.EncryptString(lead.Phone)

        if emailSuppressionSet[emailHash] {
            validationErrors = append(validationErrors, fmt.Sprintf("Row %d: Lead suppressed — email matches suppression list", row))
            continue
        }
        if phoneSuppressionSet[phoneHash] {
            validationErrors = append(validationErrors, fmt.Sprintf("Row %d: Lead suppressed — phone matches suppression list", row))
            continue
        }

        if companySet[strings.ToLower(lead.CompanyName)] {
            validationErrors = append(validationErrors, fmt.Sprintf("Row %d: Lead suppressed — company '%s' is suppressed", row, lead.CompanyName))
            continue
        }
        if regionSet[strings.ToLower(lead.Address)] { // assuming region comes from Address or separate field
            validationErrors = append(validationErrors, fmt.Sprintf("Row %d: Lead suppressed — region matches suppression list", row))
            continue
        }
        if regionSet[strings.ToLower(lead.Region)] {
            validationErrors = append(validationErrors, fmt.Sprintf("Row %d: Lead suppressed — region '%s' is suppressed", row, lead.Region))
            continue
        }
        if stateSet[strings.ToLower(lead.State)] {
            validationErrors = append(validationErrors, fmt.Sprintf("Row %d: Lead suppressed — state '%s' is suppressed", row, lead.State))
            continue
        }

        if empSizeSet[strings.ToLower(lead.EmployeeSize)] {
            validationErrors = append(validationErrors, fmt.Sprintf("Row %d: Lead suppressed — employee size matches suppression list", row))
            continue
        }
        if revenueSet[strings.ToLower(lead.RevenueSize)] {
            validationErrors = append(validationErrors, fmt.Sprintf("Row %d: Lead suppressed — revenue size matches suppression list", row))
            continue
        }
    }

    // stop if validation failed
    if len(validationErrors) > 0 {
        c.JSON(http.StatusBadRequest, gin.H{
            "error":  "Lead validation failed",
            "issues": validationErrors,
        })
        return
    }

    // insert valid leads
    count := 0
    for _, lead := range leads {
        emailHash, _ := utils.EncryptString(lead.Email)
        phoneHash, _ := utils.EncryptString(lead.Phone)
        ipParam := strings.TrimSpace(lead.IPAddress)

        _, err := cfg.DBQueries.CreateLead(c, queries.CreateLeadParams{
            TenantID:            tenantId,
            CampaignID:          campaignId,
            LeadBatchID:         uuid.NullUUID{UUID: batchId, Valid: true},
            FullName:            utils.SqlNullString(lead.FullName),
            EmailHash:           utils.SqlNullString(emailHash),
            PhoneHash:           utils.SqlNullString(phoneHash),
            Column11:            utils.SqlNullString(ipParam),
            CompanyName:         utils.SqlNullString(lead.CompanyName),
            Address:             utils.SqlNullString(lead.Address),
            CountryCode:         utils.SqlNullString(lead.CountryCode),
            LinkedinContact:     utils.SqlNullString(lead.LinkedInContact),
            LinkedinCompany:     utils.SqlNullString(lead.LinkedInCompany),
            DownloadedAssetName: utils.SqlNullString(lead.DownloadedAssetName),
            PublisherName:       utils.SqlNullString(lead.PublisherName),
            Industry:            utils.SqlNullString(lead.Industry),
            RevenueSize:         utils.SqlNullString(lead.RevenueSize),
            EmployeeSize:        utils.SqlNullString(lead.EmployeeSize),
            Region:              utils.SqlNullString(lead.Region),
            State:               utils.SqlNullString(lead.State),
        })

        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to insert lead: %v", err)})
            return
        }
        count++
    }

    err = cfg.DBQueries.UpdateTotalLeadsForBatch(c, batchId)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to update lead count: %v", err)})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "message": fmt.Sprintf("Successfully uploaded %d leads to batch %s", count, batchId.String()),
    })
}




func (cfg *ApiConfig) MergeLeadBatches(c *gin.Context) {
    header := c.GetHeader("Authorization")
    tenantId, err := utils.ExtractTokenTenantID(header)
    if err != nil {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid tenant token"})
        return
    }

    campaignIDParam := c.Param("campaign_id")
    campaignId, err := uuid.Parse(campaignIDParam)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid campaign id"})
        return
    }

    var req struct {
        BatchIDs  []uuid.UUID `json:"batch_ids" binding:"required"`
        BatchName string      `json:"batch_name" binding:"required"`
    }
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
        return
    }

    if len(req.BatchIDs) < 2 {
        c.JSON(http.StatusBadRequest, gin.H{"error": "need at least two batches to merge"})
        return
    }

    limit := int32(1000)
    offset := int32(0)
    var leads []queries.Lead

    // Collect all leads from each batch
    for _, batchId := range req.BatchIDs {
        batchLeads, err := cfg.DBQueries.ListLeadsForBatch(c, queries.ListLeadsForBatchParams{
            TenantID: tenantId,
            LeadBatchID: uuid.NullUUID{
                UUID:  batchId,
                Valid: true,
            },
            Limit:  limit,
            Offset: offset,
        })
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{
                "error": fmt.Sprintf("failed to fetch leads for batch %s", batchId.String()),
            })
            return
        }

        leads = append(leads, batchLeads...)
    }

    // Deduplicate by email_hash or phone_hash
    type leadKey struct {
        EmailHash string
        PhoneHash string
    }
    seen := make(map[leadKey]bool)
    var deduped []queries.Lead
    var duplicates []queries.Lead

    for _, lead := range leads {
        key := leadKey{
            EmailHash: lead.EmailHash.String,
            PhoneHash: lead.PhoneHash.String,
        }
        if seen[key] {
            duplicates = append(duplicates, lead)
        } else {
            seen[key] = true
            deduped = append(deduped, lead)
        }
    }

    // Create a new merged batch
    newBatch, err := cfg.DBQueries.CreateLeadBatch(c, queries.CreateLeadBatchParams{
        TenantID:   tenantId,
        CampaignID: campaignId,
        BatchName:  req.BatchName,
    })
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create merged batch", "details": err.Error()})
        return
    }

    // Insert deduped leads into the new batch
    for _, lead := range deduped {
        leadBatchID := uuid.NullUUID{UUID: newBatch.ID, Valid: true}
        _, err := cfg.DBQueries.CreateLead(c, queries.CreateLeadParams{
            TenantID:    tenantId,
            CampaignID:  campaignId,
            LeadBatchID: leadBatchID,
            FullName:    lead.FullName,
            EmailHash:   lead.EmailHash,
            PhoneHash:   lead.PhoneHash,
        })
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to insert lead: %v", err)})
            return
        }
    }

    // Update total leads for the new batch
    err = cfg.DBQueries.UpdateTotalLeadsForBatch(c, newBatch.ID)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update lead count"})
        return
    }

    // Return duplicates info to the UI
    c.JSON(http.StatusOK, gin.H{
        "message":           "Batches merged successfully",
        "merged_batch":      newBatch,
        "total_leads":       len(deduped),
        "duplicates_count":  len(duplicates),
        "duplicates":        duplicates,
        "deduped_leads":     deduped,
    })
}


