package parser

import "sync"

type namedParsePass struct {
	Name string
	Pass parsePass
}

var parsePassRegistryMu sync.RWMutex

var parsePassRegistry = []namedParsePass{
	{Name: "extract_pack", Pass: passExtractPack},
	{Name: "group_slot2_platform_infer", Pass: passGroupSlot2PlatformInfer},
	{Name: "group_subs_x2_override", Pass: passGroupSubsX2Override},
	{Name: "extract_tech_subs", Pass: passExtractTechSubs},
	{Name: "extract_tech_audio", Pass: passExtractTechAudio},
	{Name: "extract_bracket_title_aliases", Pass: passExtractBracketTitleAliases},
	{Name: "extract_tag_code_audio_lang", Pass: passExtractTagCodeAudioLang},
	{Name: "detect_season_sp_movie", Pass: passDetectSeasonSPMovie},
	{Name: "detect_episode_range_and_episode", Pass: passDetectEpisodeRangeAndEpisode},
}

func RegisterParsePass(name string, pass parsePass) {
	parsePassRegistryMu.Lock()
	defer parsePassRegistryMu.Unlock()
	parsePassRegistry = append(parsePassRegistry, namedParsePass{Name: name, Pass: pass})
}

func defaultNamedParsePasses() []namedParsePass {
	parsePassRegistryMu.RLock()
	defer parsePassRegistryMu.RUnlock()
	out := make([]namedParsePass, len(parsePassRegistry))
	copy(out, parsePassRegistry)
	return out
}

func defaultParsePasses() []parsePass {
	named := defaultNamedParsePasses()
	out := make([]parsePass, 0, len(named))
	for _, p := range named {
		out = append(out, p.Pass)
	}
	return out
}
