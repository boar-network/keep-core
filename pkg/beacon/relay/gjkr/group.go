package gjkr

import (
	"fmt"

	"github.com/keep-network/keep-core/pkg/beacon/relay/member"
)

// Group is protocol's members group.
type Group struct {
	// The maximum number of group members who could be dishonest in order for
	// the generated key to be uncompromised.
	dishonestThreshold int
	// IDs of all members of the group. Contains local member's ID.
	// Initially empty, populated as each other member announces its presence.
	memberIDs []member.Index
	// IDs of all disqualified members of the group.
	disqualifiedMemberIDs []member.Index
	// IDs of all inactive members of the group.
	inactiveMemberIDs []member.Index
}

// MemberIDs returns IDs of all group members, as initially selected to the
// group. Returned list contains IDs of all members, including those marked as
// inactive or disqualified.
func (g *Group) MemberIDs() []member.Index {
	return g.memberIDs
}

// RegisterMemberID adds a member to the list of group members.
func (g *Group) RegisterMemberID(memberID member.Index) error {
	if err := memberID.Validate(); err != nil {
		return fmt.Errorf("cannot register member ID in the group [%v]", err)
	}

	for _, id := range g.memberIDs {
		if id == memberID {
			return nil // already there
		}
	}
	g.memberIDs = append(g.memberIDs, memberID)

	return nil
}

// OperatingMemberIDs returns IDs of all group members that are active and have
// not been disqualified. All those members are properly operating in the group
// at the moment of calling this method.
func (g *Group) OperatingMemberIDs() []member.Index {
	operatingMembers := make([]member.Index, 0)
	for _, member := range g.memberIDs {
		if g.isOperating(member) {
			operatingMembers = append(operatingMembers, member)
		}
	}

	return operatingMembers
}

// MarkMemberAsDisqualified adds the member with the given ID to the list of
// disqualified members. If the member is not a part of the group, is already
// disqualified or marked as inactive, method does nothing.
func (g *Group) MarkMemberAsDisqualified(memberID member.Index) {
	if g.isOperating(memberID) {
		g.disqualifiedMemberIDs = append(g.disqualifiedMemberIDs, memberID)
	}
}

// MarkMemberAsInactive adds the member with the given ID to the list of
// inactive members. If the member is not a part of the group, is already
// disqualified or marked as inactive, method does nothing.
func (g *Group) MarkMemberAsInactive(memberID member.Index) {
	if g.isOperating(memberID) {
		g.inactiveMemberIDs = append(g.inactiveMemberIDs, memberID)
	}
}

func (g *Group) isOperating(memberID member.Index) bool {
	return g.isInGroup(memberID) &&
		!g.isInactive(memberID) &&
		!g.isDisqualified(memberID)
}

func (g *Group) isInGroup(memberID member.Index) bool {
	for _, groupMember := range g.memberIDs {
		if groupMember == memberID {
			return true
		}
	}

	return false
}

func (g *Group) isInactive(memberID member.Index) bool {
	for _, inactiveMemberID := range g.inactiveMemberIDs {
		if memberID == inactiveMemberID {
			return true
		}
	}

	return false
}

func (g *Group) isDisqualified(memberID member.Index) bool {
	for _, disqualifiedMemberID := range g.disqualifiedMemberIDs {
		if memberID == disqualifiedMemberID {
			return true
		}
	}

	return false
}

func (g *Group) eliminatedMembersCount() int {
	return len(g.disqualifiedMemberIDs) + len(g.inactiveMemberIDs)
}

// isThresholdSatisfied checks number of disqualified and inactive members in
// the group. If the number is less or equal half of dishonest threshold,
// returns true.
func (g *Group) isThresholdSatisfied() bool {
	return g.eliminatedMembersCount() <= g.dishonestThreshold/2
}
