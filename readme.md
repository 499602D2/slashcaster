## slashCaster: an Ethereum beacon chain slashing event broadcaster

SlashCaster keeps up with the head of the beacon chain, observing incoming blocks for attestation and proposer violations. If a violation is detected, the bot broadcasts the slashing through multiple channels.

### Direct dependencies
- `discordgo`
- `telebot.v2`
- `go-humanize`

### Running the program
In order to run the program, you need at least a Telegram bot API key and a token for Infura's API. These are requested when you run the program for the first time.