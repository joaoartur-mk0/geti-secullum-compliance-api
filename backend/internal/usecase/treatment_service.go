package usecase

import (
	"bytes"
	"path/filepath"
	"strings"
	"unicode"

	"backend/internal/domain"
)

// AttachmentMaxSizeBytes é o teto por arquivo (5 MB) — anexo de tratativa é PDF de
// atestado/acordo, não deveria passar disso; valor generoso o bastante para um documento
// escaneado em qualidade razoável. Exportado para o handler HTTP dimensionar o limite de
// leitura do multipart sobre o mesmo número, em vez de duplicar a constante.
const AttachmentMaxSizeBytes = 5 * 1024 * 1024

// pdfMagicBytes é a assinatura de um PDF real ("%PDF-"). O Content-Type do multipart é
// declarado pelo cliente e não prova nada sozinho — um executável renomeado com header
// falso passaria pela checagem de ContentType. Conferir a assinatura do conteúdo é o que
// de fato distingue um PDF de qualquer outro arquivo.
var pdfMagicBytes = []byte("%PDF-")

// sanitizeFileName remove tudo que não seja o nome do arquivo em si: separadores de
// caminho (contra path traversal) e caracteres de controle/aspas (contra quebra do
// header Content-Disposition no download). Preserva o nome legível para o usuário.
func sanitizeFileName(name string) string {
	name = filepath.Base(name)
	var b strings.Builder
	for _, r := range name {
		if r == '"' || r == '\\' || unicode.IsControl(r) {
			continue
		}
		b.WriteRune(r)
	}
	sanitized := strings.TrimSpace(b.String())
	if sanitized == "" {
		return "anexo.pdf"
	}
	return sanitized
}

// TreatmentService aplica as regras de negócio da Feature 4 (tratativa) antes de gravar:
// exigência de anexo por tipo, tamanho e formato do anexo. A transação em si (gravar
// tratativa + anexo + transicionar a ocorrência) é responsabilidade do repositório —
// ver domain.TreatmentRepository.Treat.
type TreatmentService struct {
	occurrenceRepo    domain.OccurrenceRepository
	treatmentRepo     domain.TreatmentRepository
	monthlyReviewRepo domain.MonthlyReviewRepository
}

func NewTreatmentService(
	occurrenceRepo domain.OccurrenceRepository,
	treatmentRepo domain.TreatmentRepository,
	monthlyReviewRepo domain.MonthlyReviewRepository,
) *TreatmentService {
	return &TreatmentService{
		occurrenceRepo:    occurrenceRepo,
		treatmentRepo:     treatmentRepo,
		monthlyReviewRepo: monthlyReviewRepo,
	}
}

// AttachmentInput é o anexo cru recebido do handler HTTP, antes de virar domain.Attachment.
type AttachmentInput struct {
	FileName    string
	ContentType string
	Data        []byte
}

// Treat valida e grava a tratativa de uma ocorrência. O tenant já foi conferido pelo
// handler (ensureTenantAccess) antes de chamar este método — mesmo padrão de Ignore.
//
// Regras validadas aqui (antes de tocar no banco):
//   - o tipo da ocorrência pode exigir anexo (TypeRequiresAttachment) — sem ele, bloqueia;
//   - quando há anexo, só PDF é aceito e até attachmentMaxSizeBytes.
//
// O restante (ocorrência já tratada/ignorada, transação atômica) é responsabilidade do
// repositório, que devolve NewConflict se a ocorrência já tiver desfecho.
func (s *TreatmentService) Treat(occurrenceID int, justification string, attachment *AttachmentInput, actorUserID int) (*domain.Treatment, error) {
	const op = "TreatmentService.Treat"

	occ, err := s.occurrenceRepo.GetByID(occurrenceID)
	if err != nil {
		return nil, err
	}

	if err := domain.EnsureCompetenciaAberta(s.monthlyReviewRepo, op, occ.TenantID, occ.Date, "tratar"); err != nil {
		return nil, err
	}

	if justification == "" {
		return nil, domain.NewValidation(op, "justificativa obrigatória", nil).
			WithDetails("toda tratativa precisa de uma justificativa")
	}

	requiresAttachment := TypeRequiresAttachment(occ.Type)
	if requiresAttachment && attachment == nil {
		return nil, domain.NewValidation(op, "anexo obrigatório para este tipo", nil).
			WithDetails("o tipo '" + occ.Type + "' exige anexo na tratativa")
	}

	var attachments []domain.Attachment
	if attachment != nil {
		if attachment.ContentType != "application/pdf" {
			return nil, domain.NewValidation(op, "formato de anexo inválido", nil).
				WithDetails("só PDF é aceito como anexo de tratativa")
		}
		if !bytes.HasPrefix(attachment.Data, pdfMagicBytes) {
			return nil, domain.NewValidation(op, "conteúdo do anexo não é um PDF", nil).
				WithDetails("o arquivo enviado não tem a assinatura de um PDF válido")
		}
		if len(attachment.Data) > AttachmentMaxSizeBytes {
			return nil, domain.NewValidation(op, "anexo muito grande", nil).
				WithDetails("o anexo excede o limite de 5 MB")
		}
		attachments = append(attachments, domain.Attachment{
			TenantID:    occ.TenantID,
			FileName:    sanitizeFileName(attachment.FileName),
			ContentType: attachment.ContentType,
			SizeBytes:   len(attachment.Data),
			Data:        attachment.Data,
		})
	}

	return s.treatmentRepo.Treat(occurrenceID, justification, attachments, actorUserID)
}

// Undo desfaz uma tratativa. O tenant já foi conferido pelo handler antes de chamar.
func (s *TreatmentService) Undo(treatmentID, actorUserID int) error {
	return s.treatmentRepo.Undo(treatmentID, actorUserID)
}
