// See README for details: https://github.com/thepudds/gomoderate#readme
package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	cliutil "github.com/bluesky-social/indigo/cmd/gosky/util"
	"github.com/urfave/cli/v2"
)

var (
	pdsServer = "https://bsky.social"
	plcServer = "https://plc.directory"
)

// The normal way to specify our auth flags is as urfave/cli "global options", like:
//
//	gomoderate --my-user @me --app-key xyz list mutes
//
// However, we also define some per-command auth flags, but hide them.
// The intent is to be friendlier to people who put them in the "wrong" spot:
//
//	gomoderate list mutes --my-user @me --app-key xyz
var localUser, localAppKey, globalUser, globalAppKey string

func main() {
	// We have a separate goModerateMain to use with go-internal/testscripts.
	os.Exit(goModerateMain())
}

func goModerateMain() int {
	localAuthFlags := []cli.Flag{
		&cli.StringFlag{
			Name:        "my-user",
			Usage:       "Your Bluesky `handle` (e.g., @user1.bsky.social)",
			Hidden:      true,
			Destination: &localUser,
		},
		&cli.StringFlag{
			Name:        "app-key",
			Usage:       "An application `key` you created in the Bluesky (e.g., xj5s-fqo6-rtlm-lsrt)",
			Hidden:      true,
			Destination: &localAppKey,
		},
	}

	listFlags := []cli.Flag{
		&cli.BoolFlag{
			Name:  "verbose",
			Usage: "output usernames as well as DIDs, which are precise identifiers of user accounts",
		},
		&cli.BoolFlag{
			Name:  "oneline",
			Usage: "output on multiple lines rather than the default single line",
		},
	}

	app := &cli.App{
		Name:  "gomoderate",
		Usage: "Moderate your Bluesky experience by bulk blocking or muting",
		// TODO: consider something like: "gomoderate --my-user <@me> --app-key <key> mute <command>\n",
		UsageText: "gomoderate list <command>\n" +
			"gomoderate mute <command>\n" +
			"gomoderate block <command>",
		Flags: []cli.Flag{ // these are considered 'global', and are specified before subcommands
			&cli.StringFlag{
				Name:        "my-user",
				Usage:       "Your Bluesky `handle` (e.g., @user1.bsky.social)",
				Destination: &globalUser,
			},
			&cli.StringFlag{
				Name:        "app-key",
				Usage:       "An application `key` you created in the Bluesky (e.g., xj5s-fqo6-rtlm-lsrt)",
				Destination: &globalAppKey,
			},
		},
		CommandNotFound: func(c *cli.Context, command string) {
			// TODO: something similar for bad flags? maybe OnUsageError or InvalidFlagAccessHandler?
			msg := fmt.Sprintf("no command found matching %q", command)
			if suggestion := cli.SuggestCommand(c.Command.Subcommands, command); suggestion != "" {
				msg += ". " + suggestion
			}
			fmt.Fprintln(os.Stderr, fatalArgs(c, msg))
			os.Exit(2)
		},
		// HideHelpCommand: true, // TODO: better? worse?
		Commands: []*cli.Command{
			{
				Name:  "mute",
				Usage: "Mute users.",
				// | from-url | from-file
				UsageText: "gomoderate mute users <@user1> [@user2 ...]\n" +
					"gomoderate mute from-user-blocks @user1 [@user2 ...]\n" +
					"gomoderate mute from-file <file1> [file2 ...]\n" +
					"gomoderate mute from-url <url1> [url2 ...]",
				HideHelpCommand: true,
				Subcommands: []*cli.Command{
					{
						Name:      "users",
						Usage:     "Mute one or more specified users.",
						UsageText: "gomoderate mute users <@user1> [@user2 ...]",
						ArgsUsage: "<@user1> [@user2 ...]",
						// must be authenticated
						Flags: append(localAuthFlags, listFlags...),
						Action: func(c *cli.Context) error {
							examples := []string{"gomoderate --my-user @me.bsky.social --app-key xyz mute users @someone.bsky.social",
								"gomoderate --my-user @me.bsky.social --app-key xyz mute users @someone.bsky.social @another.user.io"}
							if c.Args().Len() < 1 {
								return fatalArgs2(c, "at least one user must be provided", examples)
							}
							xrpcc, err := newXrpcClient()
							if err != nil {
								return err
							}
							err = authenticate(xrpcc)
							if err != nil {
								return err
							}

							err = doMuteCmd(c, xrpcc, c.Args().Slice())
							if err != nil {
								return err
							}
							return nil
						},
					},
					{
						Name:      "from-user-blocks",
						Usage:     "Mute users from user blocks.",
						UsageText: "gomoderate mute from-user-blocks @user1 [@user2 ...]",
						ArgsUsage: "user1 [@user2 ...]",
						// must be authenticated
						Flags: localAuthFlags,
						Action: func(c *cli.Context) error {
							if c.Args().Len() < 1 {
								return fatalArgs(c, "at least one user must be provided")
							}
							xrpcc, err := newXrpcClient()
							if err != nil {
								return err
							}
							err = authenticate(xrpcc)
							if err != nil {
								return err
							}

							err = doMuteFromUserBlocksCmd(c, xrpcc, c.Args().Slice())
							if err != nil {
								return err
							}
							return nil
						},
					},
					{
						Name:      "from-file",
						Usage:     "Mute users from file.",
						UsageText: "gomoderate mute from-file <file1> [file2 ...]",
						ArgsUsage: "<file1> [file2 ...]",
						Action: func(c *cli.Context) error {
							if c.Args().Len() < 1 {
								return fatalArgs(c, "at least one file must be provided")
							}
							xrpcc, err := newXrpcClient()
							if err != nil {
								return err
							}
							err = authenticate(xrpcc)
							if err != nil {
								return err
							}

							filenames := c.Args().Slice()
							for _, filename := range filenames {
								f, err := os.Open(filename)
								if err != nil {
									return fmt.Errorf("mute from file: %w", err)
								}
								defer f.Close()

								dids, err := parseUserList(f)
								if err != nil {
									return fmt.Errorf("parsing %s: %w", filename, err)
								}
								err = muteUsers(xrpcc, dids)
								if err != nil {
									return fmt.Errorf("handling %s: %w", filename, err)
								}
							}
							return nil
						},
					},
					{
						Name:      "from-url",
						Usage:     "Mute users from URL.",
						UsageText: "gomoderate mute from-url <url1> [url2 ...]",
						ArgsUsage: "<url1> [url2 ...]",
						Action: func(c *cli.Context) error {
							if c.Args().Len() < 1 {
								return fatalArgs(c, "at least one URL must be provided")
							}
							xrpcc, err := newXrpcClient()
							if err != nil {
								return err
							}
							err = authenticate(xrpcc)
							if err != nil {
								return err
							}

							urls := c.Args().Slice()
							client := cliutil.NewHttpClient()
							for _, url := range urls {
								resp, err := client.Get(url)
								if err != nil {
									return fmt.Errorf("failed fetching url: %w", err)
								}
								defer resp.Body.Close()
								switch {
								case resp.StatusCode == http.StatusNotFound:
									return fmt.Errorf("resource not found: %s", url)
								case resp.StatusCode != http.StatusOK:
									return fmt.Errorf("unexpected status code %d when fetching %s", resp.StatusCode, url)
								}
								dids, err := parseUserList(resp.Body)
								if err != nil {
									return fmt.Errorf("parsing %s: %w", url, err)
								}
								err = muteUsers(xrpcc, dids)
								if err != nil {
									return fmt.Errorf("handling %s: %w", url, err)
								}
							}
							return nil
						},
					},
				},
			},
			// TODO: NYI
			// {
			// 	Name:            "block",
			// 	Usage:           "Block users.",
			// 	HideHelpCommand: true,
			// 	Subcommands: []*cli.Command{
			// 		{
			// 			Name:      "users",
			// 			Usage:     "Block specified users.",
			// 			UsageText: "gomoderate block users <@user1> [@user2 ...]",
			// 			ArgsUsage: "<@user1> [@user2 ...]",
			// 			Action: func(c *cli.Context) error {
			// 				if c.Args().Len() < 1 {
			// 					return fatalArgs(c, "at least one user must be provided")
			// 				}
			// 				return fatalArgs(c, "sorry, not yet implemented. coming soon.")
			// 			},
			// 		},
			// 		{
			// 			Name:      "from-userblocks",
			// 			Usage:     "Block users from userblocks.",
			// 			UsageText: "gomoderate block from-userblocks @user1 [@user2 ...]",
			// 			ArgsUsage: "user1 [@user2 ...]",
			// 			Action: func(c *cli.Context) error {
			// 				if c.Args().Len() < 1 {
			// 					return fatalArgs(c, "at least one user must be provided")
			// 				}
			// 				return fatalArgs(c, "sorry, not yet implemented. coming soon.")
			// 			},
			// 		},
			// 		{
			// 			Name:      "from-file",
			// 			Usage:     "Block users from file.",
			// 			UsageText: "gomoderate block from-file <file1> [file2 ...]",
			// 			ArgsUsage: "<file1> [file2 ...]",
			// 			Action: func(c *cli.Context) error {
			// 				if c.Args().Len() < 1 {
			// 					return fatalArgs(c, "at least one file must be provided")
			// 				}
			// 				return fatalArgs(c, "sorry, not yet implemented. coming soon.")
			// 			},
			// 		},
			// 		{
			// 			Name:      "from-url",
			// 			Usage:     "Block users from URL.",
			// 			UsageText: "gomoderate block from-url <url1> [url2 ...]",
			// 			ArgsUsage: "<url1> [url2 ...]",
			// 			Action: func(c *cli.Context) error {
			// 				if c.Args().Len() < 1 {
			// 					return fatalArgs(c, "at least one URL must be provided")
			// 				}
			// 				return fatalArgs(c, "sorry, not yet implemented. coming soon.")
			// 			},
			// 		},
			// 	},
			// },
			{
				Name:            "list",
				Usage:           "List mutes or blocks.",
				HideHelpCommand: true,
				Subcommands: []*cli.Command{
					{
						Name:  "mutes",
						Usage: "List mutes.",
						// must be authenticated
						Flags: append(localAuthFlags, listFlags...),
						Action: func(c *cli.Context) error {
							if c.Args().Len() > 0 {
								return fatalArgs(c, "list mutes command does not accept any arguments")
							}
							xrpcc, err := newXrpcClient()
							if err != nil {
								return err
							}
							err = authenticate(xrpcc)
							if err != nil {
								return err
							}

							err = doListMutesCmd(c, xrpcc)
							if err != nil {
								return err
							}
							return nil
						},
					},
					{
						Name:      "blocks",
						Usage:     "List blocks.",
						UsageText: "gomoderate list blocks <@user1> [@@user2 ...]",
						ArgsUsage: "<@user1> [@@user2 ...]",
						Flags:     listFlags,
						Action: func(c *cli.Context) error {
							if c.Args().Len() == 0 {
								return fatalArgs(c, "list blocks command requires at least one username, such as @user1.bsky.social")
							}
							xrpcc, err := newXrpcClient()
							if err != nil {
								return err
							}
							// no need to authenticate
							err = doListBlocksCmd(c, xrpcc, c.Args().Slice())
							if err != nil {
								return err
							}
							return nil
						},
					},
				},
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		// TODO: do some errors get printed twice if urfave/cli decides to print help? what's normal way to do this?
		// I think urfave/cli might print its default usage errors to stdout, so maybe this is ok.
		fmt.Fprintf(os.Stderr, "\nerror: %v\n", err)
		os.Exit(1)
	}
	return 0
}

func authFlags() (user string, appKey string, err error) {
	haveUser := localUser != "" || globalUser != ""
	haveAppKey := localAppKey != "" || globalAppKey != ""

	msg := "Example:\n" +
		"   gomoderate --my-user @me.bsky.social --app-key xyz mute users @someone.else\n\n" +
		"Application keys look something like xj5s-fqo6-rtfm-lsrt.\n" +
		"If you do not have one, you can create an application key\n" +
		"in the Bluesky web interface here:\n" +
		"   https://staging.bsky.app/settings/app-passwords\n"

	if !haveUser && !haveAppKey {
		return "", "", cli.Exit("Error: both the --my-user and --app-key flags must be provided with your Bluesky handle and application key.\n\n"+msg, 2)
	}
	if !haveAppKey {
		return "", "", cli.Exit("Error: the --app-key flag must be provided with an application key.\n\n"+msg, 2)
	}
	if !haveUser {
		return "", "", cli.Exit("Error: the --my-user flag must be provided with you Bluesky handle.\n\n"+msg, 2)
	}

	user = localUser
	if user == "" {
		user = globalUser
	}
	appKey = localAppKey
	if appKey == "" {
		appKey = globalAppKey
	}
	if user == "" || appKey == "" {
		panic("unexpected: missing user or app key")
	}

	// Trim any leading '@'
	if user[0] == '@' {
		user = user[1:]
	}

	return user, appKey, nil
}

// TODO: delete this, cut over to the fatalArgs with examples
func fatalArgs(c *cli.Context, msg string) error {
	// prefix usage with 3 spaces
	lines := strings.Split(c.Command.UsageText, "\n")
	for i, line := range lines {
		lines[i] = "   " + line
	}
	usage := strings.Join(lines, "\n")

	return cli.Exit(fmt.Errorf("------\nerror: %s\n------\nusage:\n%s\nhelp:\n   %s --help",
		msg,
		usage,
		c.Command.HelpName,
	), 2)
}

// TODO: cut over to this fatalArgs with examples
func fatalArgs2(c *cli.Context, msg string, examples []string) error {
	indent := func(s string) string {
		// prefix usage with 3 spaces
		lines := strings.Split(s, "\n")
		for i, line := range lines {
			lines[i] = "   " + line
		}
		return strings.Join(lines, "\n")
	}

	var out string
	if len(examples) == 0 {
		// examples are friendlier, but we don't have any,
		// so output UsageText
		out = fmt.Sprintf("------\nerror: %s\n------\nusage:\n%s\nhelp:\n   %s --help",
			msg,
			indent(c.Command.UsageText),
			c.Command.HelpName)
	} else {
		// use our examples (and not UsageText).
		out = fmt.Sprintf("------\nerror: %s\n------\nexamples:\n   %s\nhelp:\n   %s --help",
			msg,
			strings.Join(examples, "\n   "),
			c.Command.HelpName)
	}
	return cli.Exit(out, 2)
}
