package handlers

import (
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"backend/internal/domain"
	"backend/internal/interface/http/httperr"
	"backend/internal/usecase"
)

// TreatmentHandler expõe a Feature 4 (tratativa): dar desfecho real a uma ocorrência,
// com justificativa e, quando o tipo exige, anexo em PDF — ver
// docs/documento-funcional-compliance.md §7.2 e §7.3.
type TreatmentHandler struct {
	treatmentSvc   *usecase.TreatmentService
	treatmentRepo  domain.TreatmentRepository
	occurrenceRepo domain.OccurrenceRepository
	userTenantRepo domain.UserTenantRepository
}

func NewTreatmentHandler(
	treatmentSvc *usecase.TreatmentService,
	treatmentRepo domain.TreatmentRepository,
	occurrenceRepo domain.OccurrenceRepository,
	userTenantRepo domain.UserTenantRepository,
) *TreatmentHandler {
	return &TreatmentHandler{
		treatmentSvc:   treatmentSvc,
		treatmentRepo:  treatmentRepo,
		occurrenceRepo: occurrenceRepo,
		userTenantRepo: userTenantRepo,
	}
}

// attachmentMaxUploadBytes limita o corpo multipart lido em memória — um pouco acima do
// teto de negócio (usecase.AttachmentMaxSizeBytes) para sobrar espaço aos demais campos
// do formulário sem cortar o arquivo bem na borda.
const attachmentMaxUploadBytes = usecase.AttachmentMaxSizeBytes + 1024*1024

type treatmentResponse struct {
	ID             int                  `json:"id"`
	OccurrenceID   int                  `json:"occurrence_id"`
	Justification  string               `json:"justification"`
	ActorUserID    int                  `json:"actor_user_id"`
	CreatedAt      string               `json:"created_at"`
	UndoneAt       *string              `json:"undone_at"`
	UndoneByUserID *int                 `json:"undone_by_user_id"`
	Attachments    []attachmentResponse `json:"attachments"`
}

type attachmentResponse struct {
	ID          int    `json:"id"`
	FileName    string `json:"file_name"`
	ContentType string `json:"content_type"`
	SizeBytes   int    `json:"size_bytes"`
}

func toTreatmentResponse(t domain.Treatment) treatmentResponse {
	out := treatmentResponse{
		ID:             t.ID,
		OccurrenceID:   t.OccurrenceID,
		Justification:  t.Justification,
		ActorUserID:    t.ActorUserID,
		CreatedAt:      t.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UndoneByUserID: t.UndoneByUserID,
	}
	if t.UndoneAt != nil {
		s := t.UndoneAt.Format("2006-01-02T15:04:05Z07:00")
		out.UndoneAt = &s
	}
	for _, a := range t.Attachments {
		out.Attachments = append(out.Attachments, attachmentResponse{
			ID:          a.ID,
			FileName:    a.FileName,
			ContentType: a.ContentType,
			SizeBytes:   a.SizeBytes,
		})
	}
	return out
}

// Treat — POST /api/v1/occurrences/:occurrenceId/treat (multipart/form-data)
// Campos: justification (obrigatório), attachment (arquivo PDF, obrigatório para os
// tipos marcados na taxonomia — ver usecase.TypeRequiresAttachment).
func (h *TreatmentHandler) Treat(c *gin.Context) {
	const op = "TreatmentHandler.Treat"

	occurrenceID, err := idParam(c, op, "occurrenceId")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	occurrence, err := h.occurrenceRepo.GetByID(occurrenceID)
	if err != nil {
		httperr.Respond(c, err)
		return
	}
	if err := requireRole(c, h.userTenantRepo, op, occurrence.TenantID, domain.RoleGestor); err != nil {
		httperr.Respond(c, err)
		return
	}

	uid := actorUserID(c)
	if uid == nil {
		httperr.Respond(c, domain.NewForbidden(op, "usuário não identificado", nil))
		return
	}

	justification := strings.TrimSpace(c.PostForm("justification"))

	var input *usecase.AttachmentInput
	fileHeader, ferr := c.FormFile("attachment")
	if ferr == nil {
		if fileHeader.Size > attachmentMaxUploadBytes {
			httperr.Respond(c, domain.NewValidation(op, "anexo muito grande", nil).
				WithDetails("o anexo excede o limite de 5 MB"))
			return
		}
		file, err := fileHeader.Open()
		if err != nil {
			httperr.Respond(c, domain.NewValidation(op, "falha ao ler anexo", err))
			return
		}
		defer file.Close()

		data, err := io.ReadAll(io.LimitReader(file, attachmentMaxUploadBytes+1))
		if err != nil {
			httperr.Respond(c, domain.NewValidation(op, "falha ao ler anexo", err))
			return
		}
		input = &usecase.AttachmentInput{
			FileName:    fileHeader.Filename,
			ContentType: fileHeader.Header.Get("Content-Type"),
			Data:        data,
		}
	}

	treatment, err := h.treatmentSvc.Treat(occurrenceID, justification, input, *uid)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":   "tratativa registrada com sucesso",
		"treatment": toTreatmentResponse(*treatment),
		"state":     string(domain.OccurrenceTreated),
	})
}

// Undo — POST /api/v1/treatments/:treatmentId/undo
// Desfaz a tratativa e devolve a ocorrência para `aberta`. Não apaga o registro original.
func (h *TreatmentHandler) Undo(c *gin.Context) {
	const op = "TreatmentHandler.Undo"

	treatmentID, err := idParam(c, op, "treatmentId")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	treatment, err := h.treatmentRepo.GetByID(treatmentID)
	if err != nil {
		httperr.Respond(c, err)
		return
	}
	if err := requireRole(c, h.userTenantRepo, op, treatment.TenantID, domain.RoleGestor); err != nil {
		httperr.Respond(c, err)
		return
	}

	uid := actorUserID(c)
	if uid == nil {
		httperr.Respond(c, domain.NewForbidden(op, "usuário não identificado", nil))
		return
	}

	if err := h.treatmentSvc.Undo(treatmentID, *uid); err != nil {
		httperr.Respond(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "tratativa desfeita com sucesso",
		"state":   string(domain.OccurrenceOpen),
	})
}

// Treatments — GET /api/v1/occurrences/:occurrenceId/treatments
// Lista as tratativas de uma ocorrência (inclusive as desfeitas — trilha completa).
func (h *TreatmentHandler) Treatments(c *gin.Context) {
	const op = "TreatmentHandler.Treatments"

	occurrenceID, err := idParam(c, op, "occurrenceId")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	occurrence, err := h.occurrenceRepo.GetByID(occurrenceID)
	if err != nil {
		httperr.Respond(c, err)
		return
	}
	if err := ensureTenantAccess(c, h.userTenantRepo, op, occurrence.TenantID); err != nil {
		httperr.Respond(c, err)
		return
	}

	treatments, err := h.treatmentRepo.ListByOccurrence(occurrenceID)
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	out := make([]treatmentResponse, 0, len(treatments))
	for _, t := range treatments {
		out = append(out, toTreatmentResponse(t))
	}
	c.JSON(http.StatusOK, gin.H{"treatments": out, "total": len(out)})
}

// DownloadAttachment — GET /api/v1/attachments/:attachmentId/download
//
// Único caminho de acesso ao conteúdo do anexo — nunca por URL estática (ver
// docs/documento-funcional-compliance.md §7.3: anexo de tratativa é dado de saúde).
// Cada download fica registrado em AttachmentDownload.
func (h *TreatmentHandler) DownloadAttachment(c *gin.Context) {
	const op = "TreatmentHandler.DownloadAttachment"

	attachmentID, err := idParam(c, op, "attachmentId")
	if err != nil {
		httperr.Respond(c, err)
		return
	}

	attachment, err := h.treatmentRepo.GetAttachment(attachmentID)
	if err != nil {
		httperr.Respond(c, err)
		return
	}
	if err := ensureTenantAccess(c, h.userTenantRepo, op, attachment.TenantID); err != nil {
		httperr.Respond(c, err)
		return
	}

	uid := actorUserID(c)
	if uid == nil {
		httperr.Respond(c, domain.NewForbidden(op, "usuário não identificado", nil))
		return
	}
	if err := h.treatmentRepo.RecordDownload(attachmentID, *uid); err != nil {
		httperr.Respond(c, err)
		return
	}

	// FileName já vem sanitizado (sem aspas/controle) de usecase.sanitizeFileName ao
	// gravar o anexo — mas o header nunca deve confiar cegamente em dado armazenado que
	// se originou de entrada do usuário, então usamos o encoder padrão de HTTP para o
	// parâmetro em vez de concatenar a string direto no header.
	c.Writer.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": attachment.FileName}))
	c.Data(http.StatusOK, attachment.ContentType, attachment.Data)
}
