package rbacmanifest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"basaltpass-backend/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Service struct{ db *gorm.DB }

func New(db *gorm.DB) *Service { return &Service{db: db} }

func (s *Service) Submit(tenantID, appID uint, sourceClientID string, raw []byte) (*SubmitResult, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("database is required")
	}
	if tenantID == 0 || appID == 0 || sourceClientID == "" {
		return nil, conflictError("authenticated app context is required")
	}
	manifest, err := DecodeManifest(raw)
	if err != nil {
		return nil, err
	}
	canonical, digest, err := marshalCanonical(manifest)
	if err != nil {
		return nil, err
	}

	var result SubmitResult
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var app model.App
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND tenant_id = ? AND status = ?", appID, tenantID, model.AppStatusActive).
			First(&app).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return conflictError("authenticated app is missing, inactive, or outside the tenant")
			}
			return err
		}

		var client model.OAuthClient
		if err := tx.Where("client_id = ? AND app_id = ? AND is_active = ?", sourceClientID, appID, true).First(&client).Error; err != nil {
			return conflictError("authenticated OAuth client is not bound to the app")
		}

		var sameRevision model.AppRBACManifest
		if err := tx.Where("app_id = ? AND tenant_id = ? AND source_revision = ?", appID, tenantID, manifest.Revision).First(&sameRevision).Error; err == nil {
			if sameRevision.Digest == digest {
				view, viewErr := manifestView(sameRevision)
				if viewErr != nil {
					return viewErr
				}
				result = SubmitResult{Manifest: view, Created: false}
				return nil
			}
			return conflictError("revision %d already exists with a different digest", manifest.Revision)
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		var latestRevision uint64
		if err := tx.Model(&model.AppRBACManifest{}).
			Where("app_id = ? AND tenant_id = ?", appID, tenantID).
			Select("COALESCE(MAX(source_revision), 0)").Scan(&latestRevision).Error; err != nil {
			return err
		}
		if manifest.Revision <= latestRevision {
			return conflictError("revision %d is stale; latest submitted revision is %d", manifest.Revision, latestRevision)
		}

		current, err := captureSnapshot(tx, tenantID, appID, false)
		if err != nil {
			return err
		}
		_, baseDigest, err := marshalCanonical(current)
		if err != nil {
			return err
		}
		target := SnapshotFromManifest(manifest)
		diff, err := calculateDiff(tx, tenantID, appID, current, target)
		if err != nil {
			return err
		}
		diffRaw, err := json.Marshal(diff)
		if err != nil {
			return err
		}

		now := time.Now().UTC()
		record := model.AppRBACManifest{
			TenantID: tenantID, AppID: appID, SourceClientID: sourceClientID,
			SchemaVersion: manifest.SchemaVersion, SourceRevision: manifest.Revision, Digest: digest, BaseDigest: baseDigest,
			Status: model.AppRBACManifestPending, Payload: string(canonical), Diff: string(diffRaw), SubmittedAt: now,
		}
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.AppRBACManifest{}).
			Where("app_id = ? AND tenant_id = ? AND status = ? AND id <> ?", appID, tenantID, model.AppRBACManifestPending, record.ID).
			Updates(map[string]any{"status": model.AppRBACManifestSuperseded, "reviewed_at": now, "review_note": "superseded by a newer app submission"}).Error; err != nil {
			return err
		}
		if err := writeAudit(tx, client.CreatedBy, "rbac_manifest_submit", tenantID, appID, map[string]any{
			"manifest_id": record.ID, "source_client_id": sourceClientID, "source_revision": manifest.Revision, "digest": digest, "diff": diff,
		}); err != nil {
			return err
		}
		view, err := manifestView(record)
		if err != nil {
			return err
		}
		result = SubmitResult{Manifest: view, Created: true}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *Service) ListManifests(tenantID, appID uint) ([]ManifestView, error) {
	if err := s.ensureAppTenant(tenantID, appID); err != nil {
		return nil, err
	}
	var records []model.AppRBACManifest
	if err := s.db.Where("tenant_id = ? AND app_id = ?", tenantID, appID).
		Order("source_revision DESC, id DESC").Limit(100).Find(&records).Error; err != nil {
		return nil, err
	}
	views := make([]ManifestView, 0, len(records))
	for _, record := range records {
		view, err := manifestView(record)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

func (s *Service) GetManifest(tenantID, appID, manifestID uint) (*ManifestView, error) {
	var record model.AppRBACManifest
	if err := s.db.Where("id = ? AND tenant_id = ? AND app_id = ?", manifestID, tenantID, appID).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	view, err := manifestView(record)
	return &view, err
}

func (s *Service) GetManifestForClient(tenantID, appID uint, sourceClientID string, manifestID uint) (*ManifestView, error) {
	var record model.AppRBACManifest
	if err := s.db.Where("id = ? AND tenant_id = ? AND app_id = ? AND source_client_id = ?", manifestID, tenantID, appID, sourceClientID).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	view, err := manifestView(record)
	return &view, err
}

func (s *Service) Approve(tenantID, appID, manifestID, reviewerID uint) (*ManifestView, error) {
	if reviewerID == 0 {
		return nil, conflictError("reviewer is required")
	}
	var approved ManifestView
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var app model.App
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND tenant_id = ? AND status = ?", appID, tenantID, model.AppStatusActive).First(&app).Error; err != nil {
			return ErrNotFound
		}
		var record model.AppRBACManifest
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND tenant_id = ? AND app_id = ?", manifestID, tenantID, appID).First(&record).Error; err != nil {
			return ErrNotFound
		}
		if record.Status != model.AppRBACManifestPending {
			return conflictError("only pending manifests can be approved; current status is %s", record.Status)
		}
		manifest, err := DecodeManifest([]byte(record.Payload))
		if err != nil {
			return err
		}
		target := SnapshotFromManifest(manifest)
		current, err := captureSnapshot(tx, tenantID, appID, true)
		if err != nil {
			return err
		}
		_, currentDigest, err := marshalCanonical(current)
		if err != nil {
			return err
		}
		if currentDigest != record.BaseDigest {
			return conflictError("effective RBAC changed after this diff was generated; the app must submit a higher revision")
		}
		diff, err := calculateDiff(tx, tenantID, appID, current, target)
		if err != nil {
			return err
		}
		if err := ensureNoAssignmentRemovalBlocks(diff); err != nil {
			return err
		}

		if err := s.ensureBaselineRevision(tx, tenantID, appID, reviewerID, current); err != nil {
			return err
		}
		if err := applySnapshot(tx, tenantID, appID, target); err != nil {
			return err
		}
		revision, err := createActiveRevision(tx, tenantID, appID, reviewerID, "manifest", &record.ID, nil, target)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		diffRaw, _ := json.Marshal(diff)
		result := tx.Model(&model.AppRBACManifest{}).
			Where("id = ? AND status = ?", record.ID, model.AppRBACManifestPending).
			Updates(map[string]any{
				"status": model.AppRBACManifestApproved, "reviewed_at": now, "reviewed_by": reviewerID,
				"active_revision_id": revision.ID, "diff": string(diffRaw), "review_note": "approved",
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return conflictError("manifest status changed concurrently")
		}
		if err := writeAudit(tx, reviewerID, "rbac_manifest_approve", tenantID, appID, map[string]any{
			"manifest_id": record.ID, "source_revision": record.SourceRevision, "digest": record.Digest,
			"active_revision_id": revision.ID, "active_revision": revision.Revision, "diff": diff,
		}); err != nil {
			return err
		}
		if err := tx.First(&record, record.ID).Error; err != nil {
			return err
		}
		approved, err = manifestView(record)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &approved, nil
}

func (s *Service) Reject(tenantID, appID, manifestID, reviewerID uint, note string) (*ManifestView, error) {
	if reviewerID == 0 {
		return nil, conflictError("reviewer is required")
	}
	if len(note) > 500 {
		return nil, validationError("review note exceeds 500 characters")
	}
	var view ManifestView
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var record model.AppRBACManifest
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND tenant_id = ? AND app_id = ?", manifestID, tenantID, appID).First(&record).Error; err != nil {
			return ErrNotFound
		}
		if record.Status != model.AppRBACManifestPending {
			return conflictError("only pending manifests can be rejected")
		}
		now := time.Now().UTC()
		if note == "" {
			note = "rejected by tenant administrator"
		}
		if err := tx.Model(&record).Updates(map[string]any{
			"status": model.AppRBACManifestRejected, "reviewed_at": now, "reviewed_by": reviewerID, "review_note": note,
		}).Error; err != nil {
			return err
		}
		if err := writeAudit(tx, reviewerID, "rbac_manifest_reject", tenantID, appID, map[string]any{
			"manifest_id": record.ID, "source_revision": record.SourceRevision, "digest": record.Digest, "note": note,
		}); err != nil {
			return err
		}
		if err := tx.First(&record, record.ID).Error; err != nil {
			return err
		}
		var err error
		view, err = manifestView(record)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &view, nil
}

func (s *Service) ListRevisions(tenantID, appID uint) ([]RevisionView, error) {
	if err := s.ensureAppTenant(tenantID, appID); err != nil {
		return nil, err
	}
	var records []model.AppRBACRevision
	if err := s.db.Where("tenant_id = ? AND app_id = ?", tenantID, appID).Order("revision DESC").Limit(100).Find(&records).Error; err != nil {
		return nil, err
	}
	views := make([]RevisionView, 0, len(records))
	for _, record := range records {
		views = append(views, revisionView(record))
	}
	return views, nil
}

func (s *Service) Rollback(tenantID, appID, targetRevisionID, reviewerID uint) (*RevisionView, error) {
	if reviewerID == 0 {
		return nil, conflictError("reviewer is required")
	}
	var view RevisionView
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var app model.App
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND tenant_id = ? AND status = ?", appID, tenantID, model.AppStatusActive).First(&app).Error; err != nil {
			return ErrNotFound
		}
		var targetRecord model.AppRBACRevision
		if err := tx.Where("id = ? AND tenant_id = ? AND app_id = ?", targetRevisionID, tenantID, appID).First(&targetRecord).Error; err != nil {
			return ErrNotFound
		}
		if targetRecord.IsActive {
			return conflictError("revision %d is already active", targetRecord.Revision)
		}
		target, err := snapshotFromJSON(targetRecord.Snapshot)
		if err != nil {
			return err
		}
		current, err := captureSnapshot(tx, tenantID, appID, true)
		if err != nil {
			return err
		}
		diff, err := calculateDiff(tx, tenantID, appID, current, target)
		if err != nil {
			return err
		}
		if err := ensureNoAssignmentRemovalBlocks(diff); err != nil {
			return err
		}
		if err := applySnapshot(tx, tenantID, appID, target); err != nil {
			return err
		}
		revision, err := createActiveRevision(tx, tenantID, appID, reviewerID, "rollback", nil, &targetRecord.ID, target)
		if err != nil {
			return err
		}
		if err := writeAudit(tx, reviewerID, "rbac_revision_rollback", tenantID, appID, map[string]any{
			"target_revision_id": targetRecord.ID, "target_revision": targetRecord.Revision,
			"active_revision_id": revision.ID, "active_revision": revision.Revision, "diff": diff,
		}); err != nil {
			return err
		}
		view = revisionView(*revision)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &view, nil
}

func (s *Service) ensureBaselineRevision(tx *gorm.DB, tenantID, appID, creatorID uint, snapshot Snapshot) error {
	var count int64
	if err := tx.Model(&model.AppRBACRevision{}).Where("tenant_id = ? AND app_id = ?", tenantID, appID).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err := createActiveRevision(tx, tenantID, appID, creatorID, "baseline", nil, nil, snapshot)
	return err
}

func createActiveRevision(tx *gorm.DB, tenantID, appID, creatorID uint, action string, manifestID, targetRevisionID *uint, snapshot Snapshot) (*model.AppRBACRevision, error) {
	snapshotRaw, digest, err := marshalCanonical(snapshot)
	if err != nil {
		return nil, err
	}
	var latest uint64
	if err := tx.Model(&model.AppRBACRevision{}).Where("tenant_id = ? AND app_id = ?", tenantID, appID).
		Select("COALESCE(MAX(revision), 0)").Scan(&latest).Error; err != nil {
		return nil, err
	}
	if err := tx.Model(&model.AppRBACRevision{}).Where("tenant_id = ? AND app_id = ? AND is_active = ?", tenantID, appID, true).
		Update("is_active", false).Error; err != nil {
		return nil, err
	}
	record := model.AppRBACRevision{
		TenantID: tenantID, AppID: appID, Revision: latest + 1, Snapshot: string(snapshotRaw), Digest: digest,
		ManifestID: manifestID, Action: action, TargetRevisionID: targetRevisionID, IsActive: true,
		CreatedBy: creatorID, CreatedAt: time.Now().UTC(),
	}
	if err := tx.Create(&record).Error; err != nil {
		return nil, err
	}
	return &record, nil
}

func (s *Service) ensureAppTenant(tenantID, appID uint) error {
	var count int64
	if err := s.db.Model(&model.App{}).Where("id = ? AND tenant_id = ?", appID, tenantID).Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return ErrNotFound
	}
	return nil
}

func manifestView(record model.AppRBACManifest) (ManifestView, error) {
	var manifest Manifest
	if err := decodeStrictJSON([]byte(record.Payload), &manifest); err != nil {
		return ManifestView{}, err
	}
	var diff Diff
	if err := decodeStrictJSON([]byte(record.Diff), &diff); err != nil {
		return ManifestView{}, err
	}
	return ManifestView{
		ID: record.ID, TenantID: record.TenantID, AppID: record.AppID, SourceClientID: record.SourceClientID,
		SchemaVersion: record.SchemaVersion, SourceRevision: record.SourceRevision, Digest: record.Digest, BaseDigest: record.BaseDigest,
		Status: record.Status, Diff: diff, Manifest: manifest, SubmittedAt: record.SubmittedAt,
		ReviewedAt: record.ReviewedAt, ReviewedBy: record.ReviewedBy, ReviewNote: record.ReviewNote,
		ActiveRevisionID: record.ActiveRevisionID,
	}, nil
}

func revisionView(record model.AppRBACRevision) RevisionView {
	return RevisionView{
		ID: record.ID, TenantID: record.TenantID, AppID: record.AppID, Revision: record.Revision,
		Digest: record.Digest, ManifestID: record.ManifestID, Action: record.Action,
		TargetRevisionID: record.TargetRevisionID, IsActive: record.IsActive,
		CreatedBy: record.CreatedBy, CreatedAt: record.CreatedAt,
	}
}

func decodeStrictJSON(raw []byte, dest any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil {
		return err
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return errors.New("trailing JSON value")
	}
	return nil
}

func writeAudit(tx *gorm.DB, userID uint, action string, tenantID, appID uint, details map[string]any) error {
	payload := map[string]any{
		"resource_type": "app_rbac", "resource_id": fmt.Sprint(appID),
		"tenant_id": tenantID, "app_id": appID, "details": details,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return tx.Create(&model.AuditLog{UserID: userID, Action: action, Data: string(raw)}).Error
}
