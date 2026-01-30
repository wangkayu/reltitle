package parser

import "title-parser/pkg/model"

func passExtractBracketTitleAliases(tokens []model.Token, attrs *[]model.Attribute, ctx *parseContext) {
	ctx.ForcedTitle, ctx.ForcedAliases, ctx.ForcedTitleStart, ctx.ForcedTitleEnd = extractBracketTitleAliasesAndRemoveFromTokens(tokens, attrs)
}
