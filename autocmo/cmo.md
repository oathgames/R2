---
description: "UGC Pipeline — generate product videos with voiceover, post to Discord. Accepts natural language commands. /ugc to run, /ugc schedule for daily automation."
user-invocable: true
---

# UGC Pipeline

You control a UGC video generation pipeline via a portable binary. Your job is to translate the user's natural language into a structured JSON command and execute it.

## The Binary

Location: `.claude/tools/AutoCMO.exe` (Windows) or `.claude/tools/AutoCMO` (macOS)
Config: `.claude/tools/autocmo-config.json` (next to the binary)

## Command Interface

The binary accepts `--cmd '<JSON>'` with this schema:

```json
{
  "action": "generate",          // "generate" | "dry-run" | "setup"

  "script": "",                  // Custom script text — replaces auto-generated script
  "format": "9:16",             // "9:16" (vertical/TikTok), "16:9" (landscape/YouTube), "1:1" (square)
  "language": "en",             // Language code: en, es, fr, de, pt, ja, ko, etc.
  "productHook": "",            // Angle/focus for this video (overrides product description)

  "voiceId": "",                // ElevenLabs voice ID (default: Rachel)
  "voiceStyle": "",             // "casual", "energetic", "serious", "warm"
  "stability": 0.0,            // 0.0–1.0 (lower = more expressive, 0 = use default)
  "skipVoice": false,           // true = no voiceover

  "discordMessage": "",         // Custom text above the Discord embed
  "skipDiscord": false          // true = generate only, don't post
}
```

**Only include fields the user actually specified.** Omitted fields use defaults.

## How to Route

### User says nothing specific (just "/ugc" or "make a video")
```bash
.claude/tools/AutoCMO.exe
```
No `--cmd` needed — defaults handle everything.

### User gives creative direction
Parse their intent into the JSON command. Examples:

**"make a landscape video about our new dungeon update"**
```bash
.claude/tools/AutoCMO.exe --cmd '{"format":"16:9","productHook":"Explore the brand new dungeon update with challenging boss fights and exclusive loot"}'
```

**"generate a hype video with energetic voiceover, don't post to discord yet"**
```bash
.claude/tools/AutoCMO.exe --cmd '{"voiceStyle":"energetic","skipDiscord":true}'
```

**"make a spanish TikTok about PvP with this script: Experience intense PvP battles..."**
```bash
.claude/tools/AutoCMO.exe --cmd '{"language":"es","format":"9:16","script":"Experience the latest collection..."}'
```

**"video with a serious tone, no voiceover, post with message 'Review this draft'"**
```bash
.claude/tools/AutoCMO.exe --cmd '{"voiceStyle":"serious","skipVoice":true,"discordMessage":"Review this draft"}'
```

**"make 3 variations — one casual, one energetic, one serious"**
Run the binary 3 times with different `voiceStyle` values. Report each result.

### User says "schedule"
Create a daily scheduled task using `mcp__scheduled-tasks__create_scheduled_task`:
- **taskId**: `daily-ugc`
- **cronExpression**: `0 9 * * 1-5`
- **description**: Generate daily UGC video and post to Discord
- **prompt**: `Run .claude/tools/AutoCMO.exe and report the result.`

Confirm: schedule set, check with `/ugc status`, auto-expires after 7 days.

### User says "setup"
```bash
.claude/tools/AutoCMO.exe --setup
```

### User says "status"
Use `mcp__scheduled-tasks__list_scheduled_tasks` and report `daily-ugc` state.

### User says "test" or "check config"
```bash
.claude/tools/AutoCMO.exe --cmd '{"action":"dry-run"}'
```

## Translation Rules

1. **Be generous with interpretation.** "make it punchy" → `energetic`. "keep it chill" → `casual`. "professional tone" → `serious`.
2. **Infer format from context.** "TikTok" / "reel" / "short" → `9:16`. "YouTube" → `16:9`. "Instagram post" → `1:1`.
3. **Infer language from request.** "in Spanish" → `es`. "French version" → `fr`.
4. **Write good scripts.** If the user gives a topic but not a full script, write a compelling 30-60 second script for them and pass it via `script`.
5. **Always confirm what you're about to do** before running, so the user can adjust.
6. **After running**, report: success/failure, video path, whether it was posted to Discord.
