package secullum

import (
	"regexp"

	"backend/internal/domain"
)

// horarioRe extrai horários "HH:MM" de uma descrição textual de jornada.
// A API Secullum devolve a jornada apenas como texto, ex.: "Horário 08:00/12:00/14:00/18:00".
var horarioRe = regexp.MustCompile(`\d{1,2}:\d{2}`)

// parseHorario transforma a descrição textual da jornada em CollaboratorSchedule.
//   - 4+ horários: jornada com intervalo (entrada1/saída1/entrada2/saída2).
//   - 2 horários: jornada corrida (entrada1/saída1).
//   - caso contrário: não é possível interpretar, retorna ok=false.
func parseHorario(descricao string) (domain.CollaboratorSchedule, bool) {
	matches := horarioRe.FindAllString(descricao, -1)

	switch {
	case len(matches) >= 4:
		return domain.CollaboratorSchedule{
			Entrada1: matches[0],
			Saida1:   matches[1],
			Entrada2: matches[2],
			Saida2:   matches[3],
		}, true
	case len(matches) == 2:
		return domain.CollaboratorSchedule{
			Entrada1: matches[0],
			Saida1:   matches[1],
		}, true
	default:
		return domain.CollaboratorSchedule{}, false
	}
}
