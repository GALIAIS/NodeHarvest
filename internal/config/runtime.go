package config

import "fmt"

// RuntimePatch contains only settings that are safe to apply between requests/jobs.
// Listener, credentials, database, route, and worker topology changes require a restart.
type RuntimePatch struct {
	PublishMinScore           *float64 `json:"publish_min_score,omitempty"`
	PublishMaxNodes           *int     `json:"publish_max_nodes,omitempty"`
	PublishAliveOnly          *bool    `json:"publish_alive_only,omitempty"`
	PublishCacheSec           *int     `json:"publish_cache_sec,omitempty"`
	PublishMaxNodeAgeHours    *int     `json:"publish_max_node_age_hours,omitempty"`
	GovernanceDisableFailures *int     `json:"governance_disable_after_failures,omitempty"`
	GovernanceCooldownHours   *int     `json:"governance_cooldown_hours,omitempty"`
	GovernanceHQDropPercent   *float64 `json:"governance_hq_drop_percent,omitempty"`
	GovernanceCountryShare    *float64 `json:"governance_country_share_percent,omitempty"`
	DialAfterQuality          *bool    `json:"dial_after_quality,omitempty"`
	DialAfterQualityMax       *int     `json:"dial_after_quality_max,omitempty"`
}

func ApplyRuntimePatch(current *Config, patch RuntimePatch) (*Config, error) {
	if current == nil {
		return nil, fmt.Errorf("config is required")
	}
	next := *current
	if patch.PublishMinScore != nil {
		if *patch.PublishMinScore < 0 || *patch.PublishMinScore > 100 {
			return nil, fmt.Errorf("publish_min_score must be between 0 and 100")
		}
		next.Publish.MinScore = *patch.PublishMinScore
	}
	if patch.PublishMaxNodes != nil {
		if *patch.PublishMaxNodes < 1 || *patch.PublishMaxNodes > 100000 {
			return nil, fmt.Errorf("publish_max_nodes must be between 1 and 100000")
		}
		next.Publish.MaxNodes = *patch.PublishMaxNodes
	}
	if patch.PublishAliveOnly != nil {
		next.Publish.AliveOnly = *patch.PublishAliveOnly
	}
	if patch.PublishCacheSec != nil {
		if *patch.PublishCacheSec < 0 || *patch.PublishCacheSec > 86400 {
			return nil, fmt.Errorf("publish_cache_sec must be between 0 and 86400")
		}
		next.Publish.CacheSec = *patch.PublishCacheSec
	}
	if patch.PublishMaxNodeAgeHours != nil {
		if *patch.PublishMaxNodeAgeHours < 1 || *patch.PublishMaxNodeAgeHours > 24*365 {
			return nil, fmt.Errorf("publish_max_node_age_hours must be between 1 and 8760")
		}
		next.Publish.MaxNodeAgeHours = *patch.PublishMaxNodeAgeHours
	}
	if patch.GovernanceDisableFailures != nil {
		if *patch.GovernanceDisableFailures < 1 || *patch.GovernanceDisableFailures > 100 {
			return nil, fmt.Errorf("governance_disable_after_failures must be between 1 and 100")
		}
		next.Governance.DisableAfterFailures = *patch.GovernanceDisableFailures
	}
	if patch.GovernanceCooldownHours != nil {
		if *patch.GovernanceCooldownHours < 1 || *patch.GovernanceCooldownHours > 24*30 {
			return nil, fmt.Errorf("governance_cooldown_hours must be between 1 and 720")
		}
		next.Governance.CooldownHours = *patch.GovernanceCooldownHours
	}
	if patch.GovernanceHQDropPercent != nil {
		if *patch.GovernanceHQDropPercent <= 0 || *patch.GovernanceHQDropPercent > 100 {
			return nil, fmt.Errorf("governance_hq_drop_percent must be between 0 and 100")
		}
		next.Governance.HQDropPercent = *patch.GovernanceHQDropPercent
	}
	if patch.GovernanceCountryShare != nil {
		if *patch.GovernanceCountryShare <= 0 || *patch.GovernanceCountryShare > 100 {
			return nil, fmt.Errorf("governance_country_share_percent must be between 0 and 100")
		}
		next.Governance.CountrySharePercent = *patch.GovernanceCountryShare
	}
	if patch.DialAfterQuality != nil {
		next.Dial.AfterQuality = *patch.DialAfterQuality
	}
	if patch.DialAfterQualityMax != nil {
		if *patch.DialAfterQualityMax < 0 || *patch.DialAfterQualityMax > 100000 {
			return nil, fmt.Errorf("dial_after_quality_max must be between 0 and 100000")
		}
		next.Dial.AfterQualityMax = *patch.DialAfterQualityMax
	}
	if err := next.Validate(); err != nil {
		return nil, err
	}
	return &next, nil
}
