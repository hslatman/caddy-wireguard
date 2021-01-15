// Copyright 2020 The gVisor Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tcp

import (
	"time"

	"gvisor.dev/gvisor/pkg/tcpip/seqnum"
)

// RACK is a loss detection algorithm used in TCP to detect packet loss and
// reordering using transmission timestamp of the packets instead of packet or
// sequence counts. To use RACK, SACK should be enabled on the connection.

// rackControl stores the rack related fields.
// See: https://tools.ietf.org/html/draft-ietf-tcpm-rack-08#section-6.1
//
// +stateify savable
type rackControl struct {
	// dsackSeen indicates if the connection has seen a DSACK.
	dsackSeen bool

	// endSequence is the ending TCP sequence number of rackControl.seg.
	endSequence seqnum.Value

	// fack is the highest selectively or cumulatively acknowledged
	// sequence.
	fack seqnum.Value

	// minRTT is the estimated minimum RTT of the connection.
	minRTT time.Duration

	// rtt is the RTT of the most recently delivered packet on the
	// connection (either cumulatively acknowledged or selectively
	// acknowledged) that was not marked invalid as a possible spurious
	// retransmission.
	rtt time.Duration

	// reorderSeen indicates if reordering has been detected on this
	// connection.
	reorderSeen bool

	// xmitTime is the latest transmission timestamp of rackControl.seg.
	xmitTime time.Time `state:".(unixTime)"`
}

// update will update the RACK related fields when an ACK has been received.
// See: https://tools.ietf.org/html/draft-ietf-tcpm-rack-08#section-7.2
func (rc *rackControl) update(seg *segment, ackSeg *segment, offset uint32) {
	rtt := time.Now().Sub(seg.xmitTime)

	// If the ACK is for a retransmitted packet, do not update if it is a
	// spurious inference which is determined by below checks:
	// 1. When Timestamping option is available, if the TSVal is less than the
	// transmit time of the most recent retransmitted packet.
	// 2. When RTT calculated for the packet is less than the smoothed RTT
	// for the connection.
	// See: https://tools.ietf.org/html/draft-ietf-tcpm-rack-08#section-7.2
	// step 2
	if seg.xmitCount > 1 {
		if ackSeg.parsedOptions.TS && ackSeg.parsedOptions.TSEcr != 0 {
			if ackSeg.parsedOptions.TSEcr < tcpTimeStamp(seg.xmitTime, offset) {
				return
			}
		}
		if rtt < rc.minRTT {
			return
		}
	}

	rc.rtt = rtt

	// The sender can either track a simple global minimum of all RTT
	// measurements from the connection, or a windowed min-filtered value
	// of recent RTT measurements. This implementation keeps track of the
	// simple global minimum of all RTTs for the connection.
	if rtt < rc.minRTT || rc.minRTT == 0 {
		rc.minRTT = rtt
	}

	// Update rc.xmitTime and rc.endSequence to the transmit time and
	// ending sequence number of the packet which has been acknowledged
	// most recently.
	endSeq := seg.sequenceNumber.Add(seqnum.Size(seg.data.Size()))
	if rc.xmitTime.Before(seg.xmitTime) || (seg.xmitTime.Equal(rc.xmitTime) && rc.endSequence.LessThan(endSeq)) {
		rc.xmitTime = seg.xmitTime
		rc.endSequence = endSeq
	}
}

// detectReorder detects if packet reordering has been observed.
// See: https://tools.ietf.org/html/draft-ietf-tcpm-rack-08#section-7.2
// * Step 3: Detect data segment reordering.
//   To detect reordering, the sender looks for original data segments being
//   delivered out of order. To detect such cases, the sender tracks the
//   highest sequence selectively or cumulatively acknowledged in the RACK.fack
//   variable. The name "fack" stands for the most "Forward ACK" (this term is
//   adopted from [FACK]). If a never retransmitted segment that's below
//   RACK.fack is (selectively or cumulatively) acknowledged, it has been
//   delivered out of order. The sender sets RACK.reord to TRUE if such segment
//   is identified.
func (rc *rackControl) detectReorder(seg *segment) {
	endSeq := seg.sequenceNumber.Add(seqnum.Size(seg.data.Size()))
	if rc.fack.LessThan(endSeq) {
		rc.fack = endSeq
		return
	}

	if endSeq.LessThan(rc.fack) && seg.xmitCount == 1 {
		rc.reorderSeen = true
	}
}

// setDSACKSeen updates rack control if duplicate SACK is seen by the connection.
func (rc *rackControl) setDSACKSeen() {
	rc.dsackSeen = true
}
