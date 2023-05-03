package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/bluesky-social/indigo/api"
	comatproto "github.com/bluesky-social/indigo/api/atproto"
	"github.com/bluesky-social/indigo/api/bsky"
	cliutil "github.com/bluesky-social/indigo/cmd/gosky/util"
	"github.com/bluesky-social/indigo/repo"
	"github.com/bluesky-social/indigo/xrpc"
	"github.com/ipfs/go-cid"
	"github.com/polydawn/refmt/cbor"
	rejson "github.com/polydawn/refmt/json"
	"github.com/polydawn/refmt/shared"
	"github.com/thepudds/bluesky-aux/appkey"
	"github.com/urfave/cli/v2"
	"golang.org/x/exp/slices"
)

// newXrpcClient returns an unauthenticated client
func newXrpcClient() (*xrpc.Client, error) {
	xrpcc := &xrpc.Client{
		Client: cliutil.NewHttpClient(),
		Host:   pdsServer,
		Auth:   nil,
	}
	return xrpcc, nil
}

// authenticate authenticates an xrpc.Client
func authenticate(xrpcc *xrpc.Client) error {
	user, appKey, err := authFlags()
	if err != nil {
		return err // don't wrap this error
	}

	ses, err := comatproto.ServerCreateSession(context.TODO(), xrpcc, &comatproto.ServerCreateSession_Input{
		Identifier: user,
		Password:   appKey,
	})
	if err != nil {
		return fmt.Errorf("authenticate: %w", err)
	}

	// validate this is a app key, not master pw
	err = appkey.Check(ses)
	if err != nil {
		return fmt.Errorf("authenticate: %w", err)
	}

	xrpcc.Auth = &xrpc.AuthInfo{
		AccessJwt:  ses.AccessJwt,
		RefreshJwt: ses.RefreshJwt,
		Handle:     ses.Handle,
		Did:        ses.Did,
	}
	return nil
}

type resolvedUser struct {
	handle string // should not include leading @. consumers add if needed.
	did    string // should be prefixed with "did"
}

func doListMutesCmd(c *cli.Context, xrpcc *xrpc.Client) error {
	printHeader(c, "users my account has muted", nil)

	resolvedUsers, err := listMutes(xrpcc)
	if err != nil {
		return err
	}

	printResolvedUsers(c, resolvedUsers)
	return nil
}

func doMuteCmd(c *cli.Context, xrpcc *xrpc.Client, handles []string) error {
	fmt.Println("muting...")
	resolvedUsers, err := resolveHandles(xrpcc, trimAts(handles))
	if err != nil {
		return fmt.Errorf("muting: %w", err)
	}
	err = muteUsers(xrpcc, didsFromUsers(resolvedUsers))
	if err != nil {
		return err
	}
	return nil
}

func doMuteFromUserBlocksCmd(c *cli.Context, xrpcc *xrpc.Client, handles []string) error {
	ctx := context.TODO()
	fmt.Println("getting blocks set by the supplied users...")
	resolvedUsers, err := resolveHandles(xrpcc, trimAts(handles))
	if err != nil {
		return fmt.Errorf("muting from user blocks: %w", err)
	}

	blockedUsers, err := listBlocks(ctx, xrpcc, resolvedUsers)
	if err != nil {
		return err
	}
	if len(blockedUsers) == 0 {
		fmt.Println("no blocks found")
		return nil
	}

	err = muteUsers(xrpcc, didsFromUsers(blockedUsers))
	if err != nil {
		return err
	}
	return nil
}

// TODO: dids should be usernames, probably with @ and error if @ missing.
func doListBlocksCmd(c *cli.Context, xrpcc *xrpc.Client, handles []string) error {
	ctx := context.TODO()

	printHeader(c, "users blocked", handles)

	resolvedUsers, err := resolveHandles(xrpcc, trimAts(handles))
	if err != nil {
		return fmt.Errorf("list blocks: %w", err)
	}

	blockedUsers, err := listBlocks(ctx, xrpcc, resolvedUsers)
	if err != nil {
		return err
	}
	// Emit ~nicely formatted results.
	printResolvedUsers(c, blockedUsers)

	// Done!
	if len(blockedUsers) == 0 {
		fmt.Println("no blocked users found")
	}
	return nil
}

func listMutes(xrpcc *xrpc.Client) ([]resolvedUser, error) {
	var resolvedUsers []resolvedUser
	var cursor string
	for {
		mutes, err := bsky.GraphGetMutes(context.TODO(), xrpcc, cursor, 100)
		if err != nil {
			return nil, fmt.Errorf("list mutes: %w", err)
		}

		for _, f := range mutes.Mutes {
			// TODO: consider flag to also include DisplayName
			resolvedUsers = append(resolvedUsers, resolvedUser{handle: f.Handle, did: f.Did})
		}
		// fmt.Println("cursor:", cursor)
		if mutes.Cursor == nil {
			break
		}
		cursor = *mutes.Cursor
	}
	return resolvedUsers, nil
}

func resolveHandles(xrpcc *xrpc.Client, handles []string) ([]resolvedUser, error) {
	ctx := context.TODO()
	var result []resolvedUser
	for _, handle := range handles {
		out, err := comatproto.IdentityResolveHandle(ctx, xrpcc, handle)
		// TODO: consider allowing partial results?
		if err != nil {
			return nil, fmt.Errorf("resolve handles: %v: %w", handle, err)
		}
		result = append(result, resolvedUser{handle: handle, did: out.Did})
	}
	return result, nil
}

func resolveDids(dids []string) ([]resolvedUser, error) {
	ctx := context.TODO()
	s := &api.PLCServer{ // TODO: probably reuse this?
		Host: plcServer,
	}
	var result []resolvedUser
	for _, did := range dids {
		doc, err := s.GetDocument(ctx, did)
		if err != nil {
			return nil, err
		}
		if len(doc.AlsoKnownAs) == 0 {
			// TODO: probably get all AlsoKnownAs?
			continue
		}
		handle := doc.AlsoKnownAs[0]
		// TODO: should we confirm "at://" is present?
		handle = strings.TrimPrefix(handle, "at://")
		result = append(result, resolvedUser{handle: handle, did: did})
	}
	return result, nil
}

func muteUsers(xrpcc *xrpc.Client, dids []string) error {
	// don't mute users that are already muted. might be friendlier to the server?
	alreadyMuted, err := listMutes(xrpcc)
	if err != nil {
		return fmt.Errorf("check for already muted users: %w", err)
	}
	alreadyMutedDids := didsFromUsers(alreadyMuted)

	// TODO: we should subtract based on dids, not did & handle
	notYetMuted := subtract(dids, alreadyMutedDids)
	switch {
	case len(notYetMuted) == 0:
		fmt.Printf("all %d users already muted, nothing more to do\n", len(dids))
		return nil
	case len(dids)-len(notYetMuted) > 0:
		fmt.Printf("%d of %d users already muted\n", len(dids)-len(notYetMuted), len(dids))
	}

	for _, did := range notYetMuted {
		// fmt.Println("muting did:", u.did)
		err := bsky.GraphMuteActor(context.TODO(),
			xrpcc,
			&bsky.GraphMuteActor_Input{Actor: did})
		if err != nil {
			return fmt.Errorf("failed to mute: %s: %w", did, err)
		}
	}
	fmt.Printf("successfully muted %d users\n", len(notYetMuted))
	return nil
}

func listBlocks(ctx context.Context, xrpcc *xrpc.Client, resolvedUsers []resolvedUser) (blockedUsers []resolvedUser, err error) {
	seenDids := make(map[string]bool)
	for _, u := range resolvedUsers {
		var blockedDids []string
		repob, err := comatproto.SyncGetRepo(ctx, xrpcc, u.did, "", "")
		if err != nil {
			return nil, fmt.Errorf("list blocks for %v: %w", u.did, err)
		}

		rr, err := repo.ReadRepoFromCar(ctx, bytes.NewReader(repob))
		if err != nil {
			return nil, fmt.Errorf("list blocks for %v: %w", u.did, err)
		}

		// get the blocks
		collection := "app.bsky.graph.block"
		err = rr.ForEach(context.TODO(), collection, func(k string, v cid.Cid) error {
			if !strings.HasPrefix(k, collection) {
				return repo.ErrDoneIterating
			}
			// fmt.Print("k: ", k, "  ")
			b, err := rr.Blockstore().Get(ctx, v)
			if err != nil {
				return fmt.Errorf("list blocks for %v: %w", u.did, err)
			}

			// TODO: probably rookie mistake, but for now, convert from cbor to json
			// and pull what we need out of the json
			convb, err := cborToJson(b.RawData())
			if err != nil {
				return fmt.Errorf("list blocks for %v: %w", u.did, err)
			}
			// fmt.Println(string(convb))

			var data map[string]any
			err = json.Unmarshal(convb, &data)
			if err != nil {
				return fmt.Errorf("list blocks for %v: %w", u.did, err)
			}
			did, ok := data["subject"].(string)
			if !ok {
				return fmt.Errorf("unexpected blocked subject %T: %v", data["subject"], data["subject"])
			}

			// dedup and store
			if !seenDids[did] {
				// TODO: add a test that sees duplicate dids
				blockedDids = append(blockedDids, did)
				seenDids[did] = true
			}
			return nil
		})

		// Done getting the blocks for this user.
		// TODO: consider emitting partial results when error (here, below)?
		if err != nil {
			return nil, fmt.Errorf("list blocks for %v: %w", u.did, err)
		}

		// TODO: resolveDids might be more expensive than some other things?
		resolvedUsers, err := resolveDids(blockedDids)
		if err != nil {
			return nil, fmt.Errorf("list blocks for %v: %w", u.did, err)
		}
		blockedUsers = append(blockedUsers, resolvedUsers...)
	}
	return blockedUsers, nil
}

func didsFromUsers(users []resolvedUser) (dids []string) {
	for _, u := range users {
		dids = append(dids, u.did)
	}
	return dids
}

func trimAts(handles []string) []string {
	var res []string
	for _, h := range handles {
		res = append(res, strings.TrimPrefix(h, "@"))
	}
	return res
}

func printHeader(c *cli.Context, header string, handles []string) {
	if !c.Bool("verbose") {
		// DIDs are what make the output useful for sharing and re-use by gomoderate (currently, anyway).
		// No header if we include DIDs -- keep it clean in case this output is stored to file and reused.
		// TODO: maybe no header for --oneline too? It's non-default, and they did ask for one line...
		msgHandles := slices.Clone(handles)
		if len(msgHandles) > 2 {
			msgHandles = append(msgHandles[:2], "...")
		}
		by := " by "
		if len(handles) == 0 {
			by = ""
		}
		fmt.Printf("\n%s%s%s", header, by, strings.Join(msgHandles, ", "))
	}
	// Finish up the header.
	switch {
	case c.Bool("oneline"):
		fmt.Print(":\n\n")
	case !c.Bool("verbose"):
		fmt.Print("\n", strings.Repeat("-", 60), "\n")
	case c.Bool("verbose"):
		// Keep it clean.
	}
}

func printResolvedUsers(c *cli.Context, resolvedUsers []resolvedUser) {
	for i, u := range resolvedUsers {
		switch {
		case c.Bool("oneline"):
			if i > 0 {
				fmt.Print(" ")
			}
			fmt.Print("@" + u.handle)
		case c.Bool("verbose"):
			// TODO: display name?
			fmt.Println(u.did, "@"+u.handle)
		default:
			fmt.Println("@" + u.handle)
		}
	}
}

func parseUserList(r io.Reader) (dids []string, err error) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		var did string
		for i := range line {
			if line[i] == ' ' || line[i] == '\t' {
				did = line[:i]
				break
			}
		}
		if did == "" {
			did = line
		}
		if !strings.HasPrefix(did, "did:plc:") {
			return nil, fmt.Errorf("bad DID in go-mod-user-list on line: %s", line)
		}
		dids = append(dids, did)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return dids, nil
}

// borrowed from indigo/gosky
func cborToJson(data []byte) ([]byte, error) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("panic: ", r)
			fmt.Printf("bad blob: %x\n", data)
		}
	}()
	buf := new(bytes.Buffer)
	enc := rejson.NewEncoder(buf, rejson.EncodeOptions{})

	dec := cbor.NewDecoder(cbor.DecodeOptions{}, bytes.NewReader(data))
	err := shared.TokenPump{TokenSource: dec, TokenSink: enc}.Run()
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// subtract does an order preserving set subtraction,
// removing from a any common elements in b.
func subtract[T comparable](a, b []T) []T {
	res := []T{}
	inB := map[T]bool{}
	for _, v := range b {
		inB[v] = true
	}
	for _, v := range a {
		if !inB[v] {
			res = append(res, v)
		}
	}
	return res
}

// func stringOrNone(s *string) string {
// 	if s == nil {
// 		return "none"
// 	}
// 	return *s
// }
