// Package contracts defines the vocabulary shared across bounded contexts:
// events, commands and the control types common to every module.
//
// Nothing here holds business rules — only the types core and modules need to
// speak in common. It is the system's shared language.
package contracts

import "time"

// ControlMode is the operating mode of a device or zone.
//
// ADR-0006: auto/manual is NOT an irrigation concept. It is a core concept,
// implemented once and inherited by every module. If each module reimplemented
// it we would end up with N subtly different semantics and a new class of bugs
// with every feature.
type ControlMode string

const (
	// ModeAuto: automation (hub rules or firmware) is in charge.
	ModeAuto ControlMode = "auto"

	// ModeManualOverride: the user temporarily suspended automation.
	// ALWAYS paired with OverrideUntil — an override without an expiry is how
	// the system forgets it is in manual and the plant dies.
	ModeManualOverride ControlMode = "manual_override"

	// ModeManual: persistent manual mode, cleared only by explicit user action.
	ModeManual ControlMode = "manual"

	// ModeDisabled: neither automation nor commands. Used during maintenance.
	ModeDisabled ControlMode = "disabled"
)

// CommandSource identifies WHO originated a command.
//
// It serves two equally critical purposes:
//  1. Arbitration — deciding who wins when two sources disagree.
//  2. Auditing — answering "who opened that valve at 3 a.m.?".
type CommandSource string

const (
	SourceFailsafe    CommandSource = "failsafe"     // level 0 — sovereign
	SourceDeviceLocal CommandSource = "device_local" // level 1 — firmware autonomy
	SourceSchedule    CommandSource = "schedule"     // level 2 — hub scheduling
	SourceRuleEngine  CommandSource = "rule_engine"  // level 2 — hub rules
	SourceUserWeb     CommandSource = "user_web"     // level 3 — user intent
	SourceUserMobile  CommandSource = "user_mobile"  // level 3 — user intent
)

// Authority is the level in the Control Authority Hierarchy (ADR-0006).
//
// HIGHER number = expresses intent. LOWER number = guarantees safety.
// The inviolable rule: the failsafe (level 0) is never overridden by anyone.
// Not by the user, not by the app, not by the cloud. That is what keeps a
// software bug from flooding the house.
type Authority uint8

const (
	AuthorityFailsafe Authority = 0
	AuthorityDevice   Authority = 1
	AuthorityHub      Authority = 2
	AuthorityUser     Authority = 3
)

// Authority maps a command source to its level in the hierarchy.
func (s CommandSource) Authority() Authority {
	switch s {
	case SourceFailsafe:
		return AuthorityFailsafe
	case SourceDeviceLocal:
		return AuthorityDevice
	case SourceSchedule, SourceRuleEngine:
		return AuthorityHub
	case SourceUserWeb, SourceUserMobile:
		return AuthorityUser
	default:
		return AuthorityUser
	}
}

// CommandKind distinguishes the two command types decided in ADR-0006.
//
// This separation is what keeps arbitration simple: "water for 30s now" and
// "suspend automation until tomorrow" are DIFFERENT command kinds, not the same
// command with a flag. Without it, the arbitration policy degenerates into a
// five-level nested if by around Sprint 6.
type CommandKind string

const (
	// KindPulse is a one-shot action. It does NOT change the control mode:
	// automation stays active once the action completes.
	KindPulse CommandKind = "pulse"

	// KindModeChange changes the control mode, optionally with a TTL.
	KindModeChange CommandKind = "mode_change"
)

// ModeState is the control state persisted in the device shadow (SH-002).
type ModeState struct {
	Mode ControlMode `json:"mode"`
	// OverrideUntil is mandatory when Mode == ModeManualOverride.
	OverrideUntil *time.Time    `json:"override_until,omitempty"`
	SetBy         CommandSource `json:"set_by"`
	SetAt         time.Time     `json:"set_at"`
}

// IsExpired reports whether a temporary override has elapsed and the device
// should fall back to ModeAuto.
func (m ModeState) IsExpired(now time.Time) bool {
	if m.Mode != ModeManualOverride || m.OverrideUntil == nil {
		return false
	}
	return now.After(*m.OverrideUntil)
}
