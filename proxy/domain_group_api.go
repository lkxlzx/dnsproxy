package proxy

import "fmt"

// EnableDomainGroup enables a domain group by name.
// Returns an error if the group is not found or domain group manager is not initialized.
func (p *Proxy) EnableDomainGroup(groupName string) error {
	if p.domainGroupMgr == nil {
		return fmt.Errorf("domain group manager not initialized")
	}

	return p.domainGroupMgr.EnableGroup(groupName)
}

// DisableDomainGroup disables a domain group by name.
// Returns an error if the group is not found or domain group manager is not initialized.
func (p *Proxy) DisableDomainGroup(groupName string) error {
	if p.domainGroupMgr == nil {
		return fmt.Errorf("domain group manager not initialized")
	}

	return p.domainGroupMgr.DisableGroup(groupName)
}

// ReloadDomainGroup reloads the domain list for a specific group.
// Returns an error if the group is not found or domain group manager is not initialized.
func (p *Proxy) ReloadDomainGroup(groupName string) error {
	if p.domainGroupMgr == nil {
		return fmt.Errorf("domain group manager not initialized")
	}

	return p.domainGroupMgr.ReloadGroup(groupName)
}

// GetDomainGroups returns the status of all domain groups.
// Returns nil if domain group manager is not initialized.
func (p *Proxy) GetDomainGroups() []DomainGroupStatus {
	if p.domainGroupMgr == nil {
		return nil
	}

	return p.domainGroupMgr.GetGroups()
}
