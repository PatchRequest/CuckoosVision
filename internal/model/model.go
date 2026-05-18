package model

import (
	"strings"
	"time"
)

type ResourceType int

const (
	ResourceScript ResourceType = iota
	ResourceStylesheet
	ResourceIframe
	ResourceImage
	ResourceLink
	ResourceMedia
	ResourceObject
	ResourceOther
)

func (r ResourceType) String() string {
	switch r {
	case ResourceScript:
		return "script"
	case ResourceStylesheet:
		return "stylesheet"
	case ResourceIframe:
		return "iframe"
	case ResourceImage:
		return "image"
	case ResourceLink:
		return "link"
	case ResourceMedia:
		return "media"
	case ResourceObject:
		return "object"
	default:
		return "other"
	}
}

func (r ResourceType) MarshalText() ([]byte, error) {
	return []byte(r.String()), nil
}

func (r *ResourceType) UnmarshalText(b []byte) error {
	switch string(b) {
	case "script":
		*r = ResourceScript
	case "stylesheet":
		*r = ResourceStylesheet
	case "iframe":
		*r = ResourceIframe
	case "image":
		*r = ResourceImage
	case "link":
		*r = ResourceLink
	case "media":
		*r = ResourceMedia
	case "object":
		*r = ResourceObject
	default:
		*r = ResourceOther
	}
	return nil
}

func (r ResourceType) RiskLevel() RiskLevel {
	switch r {
	case ResourceScript, ResourceIframe:
		return RiskCritical
	case ResourceStylesheet:
		return RiskHigh
	case ResourceImage, ResourceMedia:
		return RiskMedium
	default:
		return RiskLow
	}
}

type RiskLevel int

const (
	RiskCritical RiskLevel = iota
	RiskHigh
	RiskMedium
	RiskLow
)

func (r RiskLevel) String() string {
	switch r {
	case RiskCritical:
		return "CRITICAL"
	case RiskHigh:
		return "HIGH"
	case RiskMedium:
		return "MEDIUM"
	case RiskLow:
		return "LOW"
	default:
		return "UNKNOWN"
	}
}

func (r RiskLevel) MarshalText() ([]byte, error) {
	return []byte(r.String()), nil
}

func (r *RiskLevel) UnmarshalText(b []byte) error {
	switch strings.ToUpper(string(b)) {
	case "CRITICAL":
		*r = RiskCritical
	case "HIGH":
		*r = RiskHigh
	case "MEDIUM":
		*r = RiskMedium
	case "LOW":
		*r = RiskLow
	}
	return nil
}

func ParseRiskLevel(s string) (RiskLevel, bool) {
	switch strings.ToLower(s) {
	case "critical":
		return RiskCritical, true
	case "high":
		return RiskHigh, true
	case "medium":
		return RiskMedium, true
	case "low":
		return RiskLow, true
	default:
		return 0, false
	}
}

type Reference struct {
	SourceURL    string       `json:"source_url"`
	TargetURL    string       `json:"target_url"`
	Domain       string       `json:"domain"`
	ResourceType ResourceType `json:"resource_type"`
	Element      string       `json:"element"`
	Attribute    string       `json:"attribute"`
}

type DomainStatus struct {
	Domain    string    `json:"domain"`
	Available bool      `json:"available"`
	CheckedAt time.Time `json:"checked_at"`
}

type Finding struct {
	Domain     string      `json:"domain"`
	References []Reference `json:"references"`
	FirstSeen  time.Time   `json:"first_seen"`
	RiskLevel  RiskLevel   `json:"risk_level"`
}
