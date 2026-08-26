package plugins

import (
	cliutils "whatsrook/cmd/utils"
)

func init() {
	Register(&Command{
		Name:        "fact",
		Alias:       "facts",
		Description: "Get an interesting random fact from a public API",
		Category:    "fun",
		IsPublic:    true,
		Handler: func(ctx *Context) error {
			return ctx.Reply("💡 *Fact:* " + cliutils.GetRandomFact(ctx.Ctx))
		},
	})

	Register(&Command{
		Name:        "quotes",
		Alias:       "randomquote",
		Description: "Get an inspirational quote from a public API",
		Category:    "fun",
		IsPublic:    true,
		Handler: func(ctx *Context) error {
			return ctx.Reply("💬 " + cliutils.GetRandomQuote(ctx.Ctx))
		},
	})

	Register(&Command{
		Name:        "joke",
		Alias:       "jokes",
		Description: "Get a funny joke from a public API",
		Category:    "fun",
		IsPublic:    true,
		Handler: func(ctx *Context) error {
			return ctx.Reply("😂 " + cliutils.GetRandomJoke(ctx.Ctx))
		},
	})

	Register(&Command{
		Name:        "rizz",
		Alias:       "pickup",
		Description: "Get a smooth pickup line / rizz from a public API",
		Category:    "fun",
		IsPublic:    true,
		Handler: func(ctx *Context) error {
			return ctx.Reply("😏 " + cliutils.GetRandomRizz(ctx.Ctx))
		},
	})
}
