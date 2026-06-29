package rlsave

import "fmt"

type DoctorSeverity string

const (
	SeverityOK      DoctorSeverity = "ok"
	SeverityInfo    DoctorSeverity = "info"
	SeverityWarning DoctorSeverity = "warning"
	SeverityError   DoctorSeverity = "error"
)

type DoctorFinding struct {
	Severity DoctorSeverity `json:"severity"`
	Path     string         `json:"path,omitempty"`
	Kind     Kind           `json:"kind,omitempty"`
	Message  string         `json:"message"`
}

func DiagnoseSave(save *Save) []DoctorFinding {
	if save == nil {
		return []DoctorFinding{{Severity: SeverityError, Message: "nil save"}}
	}

	var findings []DoctorFinding
	add := func(severity DoctorSeverity, message string) {
		findings = append(findings, DoctorFinding{
			Severity: severity,
			Path:     save.Path,
			Kind:     save.Kind,
			Message:  message,
		})
	}

	stats := save.BodyStats()
	add(SeverityOK, fmt.Sprintf("kind=%s container=%s label=%q header=%d body=%d", save.Kind, save.Container, save.Label, save.HeaderLen, len(save.Body)))
	if trailing := len(save.Body) % 4; trailing != 0 {
		add(SeverityInfo, fmt.Sprintf("body has %d trailing byte(s) after the dword table; they are preserved on rebuild", trailing))
	}
	if save.Container == ContainerCompressed && save.CompressedSize <= 0 {
		add(SeverityWarning, "compressed save has no compressed body size")
	}

	switch save.Kind {
	case KindGlobal:
		if len(save.Body) < GlobalIntCount*4 {
			add(SeverityError, fmt.Sprintf("global body too small for intG table: %d bytes", len(save.Body)))
			return findings
		}
		ints, err := save.NonZeroGlobalInts()
		if err != nil {
			add(SeverityError, err.Error())
			return findings
		}
		add(SeverityInfo, fmt.Sprintf("global intG table ok: %d non-zero values", len(ints)))
	case KindRead:
		entries := save.NonZeroDWords()
		if len(entries) == 0 {
			add(SeverityInfo, "read.sav progression table is empty")
			break
		}
		last := entries[len(entries)-1]
		add(SeverityInfo, fmt.Sprintf("read.sav progression ok: %d entries, highest script seen[%d]=%d", len(entries), last.Index, last.Value))
	case KindSystem:
		add(SeverityInfo, fmt.Sprintf("system save body ok: %d non-zero dwords", stats.NonZeroDWords))
	case KindGame:
		add(SeverityInfo, fmt.Sprintf("game slot body ok: %d non-zero dwords", stats.NonZeroDWords))
	default:
		add(SeverityWarning, "save kind is unknown; low-level dword inspection only")
	}

	return findings
}
