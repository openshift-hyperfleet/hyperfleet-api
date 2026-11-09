package adapters

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
)

func voter(id, name string) EtcdMember {
	return EtcdMember{ID: id, Name: name, PeerURL: "https://" + name + ":2380"}
}

func learner(id, name string) EtcdMember {
	member := voter(id, name)
	member.Learner = true
	return member
}

func TestAddLearnerSkipsAfterSuccess(t *testing.T) {
	RegisterTestingT(t)
	client := &fakeMemberClient{members: map[string]EtcdMember{}}
	ctx := context.Background()

	acted, err := AddLearner(ctx, client, "m3", "https://m3:2380")
	Expect(err).ToNot(HaveOccurred())
	Expect(acted).To(BeTrue())
	Expect(client.calls).To(Equal([]string{"learner:m3:https://m3:2380"}))

	acted, err = AddLearner(ctx, client, "m3", "https://m3:2380")
	Expect(err).ToNot(HaveOccurred())
	Expect(acted).To(BeFalse())
	Expect(client.calls).To(HaveLen(1))
}

func TestAddLearnerSkipsWhenPeerURLExists(t *testing.T) {
	RegisterTestingT(t)
	client := &fakeMemberClient{members: map[string]EtcdMember{
		"old": voter("old", "other-name"),
	}}

	acted, err := AddLearner(
		context.Background(), client, "m3", "https://other-name:2380",
	)

	Expect(err).ToNot(HaveOccurred())
	Expect(acted).To(BeFalse())
	Expect(client.calls).To(BeEmpty())
}

func TestPromoteMemberIsIdempotent(t *testing.T) {
	RegisterTestingT(t)
	client := &fakeMemberClient{members: map[string]EtcdMember{
		"id-1": learner("id-1", "m3"),
	}}
	ctx := context.Background()

	acted, err := PromoteMember(ctx, client, "id-1")
	Expect(err).ToNot(HaveOccurred())
	Expect(acted).To(BeTrue())
	Expect(client.calls).To(Equal([]string{"promote:id-1"}))

	acted, err = PromoteMember(ctx, client, "m3")
	Expect(err).ToNot(HaveOccurred())
	Expect(acted).To(BeFalse())
	Expect(client.calls).To(HaveLen(1))
}

func TestPromoteMemberSkipsVoterAndRejectsMissingMember(t *testing.T) {
	RegisterTestingT(t)
	client := &fakeMemberClient{members: map[string]EtcdMember{
		"id-1": voter("id-1", "m0"),
	}}

	acted, err := PromoteMember(context.Background(), client, "m0")
	Expect(err).ToNot(HaveOccurred())
	Expect(acted).To(BeFalse())

	_, err = PromoteMember(context.Background(), client, "missing")
	Expect(err).To(MatchError(ContainSubstring("not found")))
}

func TestRemoveMemberIsIdempotentByIDOrName(t *testing.T) {
	RegisterTestingT(t)
	client := &fakeMemberClient{members: map[string]EtcdMember{
		"id-1": voter("id-1", "m0"),
		"id-2": voter("id-2", "m1"),
		"id-3": voter("id-3", "m2"),
	}}
	ctx := context.Background()

	acted, err := RemoveMember(ctx, client, "m1")
	Expect(err).ToNot(HaveOccurred())
	Expect(acted).To(BeTrue())
	Expect(client.calls).To(Equal([]string{"remove:id-2"}))

	acted, err = RemoveMember(ctx, client, "id-2")
	Expect(err).ToNot(HaveOccurred())
	Expect(acted).To(BeFalse())
	Expect(client.members).To(HaveLen(2))
}

func TestRemoveMemberLooksUpByID(t *testing.T) {
	RegisterTestingT(t)
	client := &fakeMemberClient{members: map[string]EtcdMember{
		"id-1": voter("id-1", "m0"),
		"id-2": voter("id-2", "m1"),
		"id-3": voter("id-3", "m2"),
		"id-4": voter("id-4", "m3"),
		"id-5": voter("id-5", "m4"),
	}}

	acted, err := RemoveMember(context.Background(), client, "id-3")

	Expect(err).ToNot(HaveOccurred())
	Expect(acted).To(BeTrue())
	Expect(client.calls).To(Equal([]string{"remove:id-3"}))
}

func TestRemoveMemberRefusesToDropQuorum(t *testing.T) {
	RegisterTestingT(t)
	client := &fakeMemberClient{members: map[string]EtcdMember{
		"id-1": voter("id-1", "m0"),
		"id-2": voter("id-2", "m1"),
	}}

	_, err := RemoveMember(context.Background(), client, "m0")

	Expect(err).To(MatchError(ContainSubstring("quorum would be lost")))
	Expect(client.calls).To(BeEmpty())
}
