package sppAdapterWeb

import (
	"fmt"
	"sync/atomic"
)

type WorkStatus struct {
	popAdult      atomic.Bool
	popMainstream atomic.Bool
	ippAdult      atomic.Bool
	ippMainstream atomic.Bool
	banAdult      atomic.Bool
	banMainstream atomic.Bool
	natAdult      atomic.Bool
	natMainstream atomic.Bool
}

func NewWorkStatus(workAdult, workMainstream bool) *WorkStatus {
	status := &WorkStatus{}
	status.Set(PostBid_POP_ADL_V_2_5_URL, workAdult)
	status.Set(PostBid_IPP_ADL_V_2_5_URL, workAdult)
	status.Set(PostBid_BAN_ADL_V_2_5_URL, workAdult)
	status.Set(PostBid_NAT_ADL_V_2_5_URL, workAdult)
	status.Set(PostBid_POP_MC_V_2_5_URL, workMainstream)
	status.Set(PostBid_IPP_MC_V_2_5_URL, workMainstream)
	status.Set(PostBid_BAN_MC_V_2_5_URL, workMainstream)
	status.Set(PostBid_NAT_MC_V_2_5_URL, workMainstream)
	return status
}

func (s *WorkStatus) Get(stream string) (bool, error) {
	if s == nil {
		return false, fmt.Errorf("work status is nil")
	}
	switch stream {
	case PostBid_POP_ADL_V_2_5_URL:
		return s.popAdult.Load(), nil
	case PostBid_POP_MC_V_2_5_URL:
		return s.popMainstream.Load(), nil
	case PostBid_IPP_ADL_V_2_5_URL:
		return s.ippAdult.Load(), nil
	case PostBid_IPP_MC_V_2_5_URL:
		return s.ippMainstream.Load(), nil
	case PostBid_BAN_ADL_V_2_5_URL:
		return s.banAdult.Load(), nil
	case PostBid_BAN_MC_V_2_5_URL:
		return s.banMainstream.Load(), nil
	case PostBid_NAT_ADL_V_2_5_URL:
		return s.natAdult.Load(), nil
	case PostBid_NAT_MC_V_2_5_URL:
		return s.natMainstream.Load(), nil
	default:
		return false, fmt.Errorf("unknown ORTB stream url %q", stream)
	}
}

func (s *WorkStatus) Set(stream string, work bool) error {
	if s == nil {
		return fmt.Errorf("work status is nil")
	}
	switch stream {
	case PostBid_POP_ADL_V_2_5_URL:
		s.popAdult.Store(work)
	case PostBid_POP_MC_V_2_5_URL:
		s.popMainstream.Store(work)
	case PostBid_IPP_ADL_V_2_5_URL:
		s.ippAdult.Store(work)
	case PostBid_IPP_MC_V_2_5_URL:
		s.ippMainstream.Store(work)
	case PostBid_BAN_ADL_V_2_5_URL:
		s.banAdult.Store(work)
	case PostBid_BAN_MC_V_2_5_URL:
		s.banMainstream.Store(work)
	case PostBid_NAT_ADL_V_2_5_URL:
		s.natAdult.Store(work)
	case PostBid_NAT_MC_V_2_5_URL:
		s.natMainstream.Store(work)
	default:
		return fmt.Errorf("unknown ORTB stream url %q", stream)
	}
	return nil
}

func (s *WorkStatus) SetAll(work bool) {
	if s == nil {
		return
	}
	s.popAdult.Store(work)
	s.popMainstream.Store(work)
	s.ippAdult.Store(work)
	s.ippMainstream.Store(work)
	s.banAdult.Store(work)
	s.banMainstream.Store(work)
	s.natAdult.Store(work)
	s.natMainstream.Store(work)
}

func (s *WorkStatus) StopAll() {
	s.SetAll(false)
}
