package auction

import "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/percenter"

// ConfigureSmartPercenter wires the complex Smart percenter into AuctionService.
// cmd/adv only assembles dependencies; pricing logic remains in the service and
// percenter packages.
func (s *AuctionService) ConfigureSmartPercenter(store *percenter.StateStore, policy percenter.Policy) {
	if s == nil {
		return
	}
	s.smartPercenter = store
	s.percenterPolicy = policy.Normalize()
}
