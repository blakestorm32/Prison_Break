# How To Play: Prison Break

## 1. Start the game

1. Start server:
   - `go run ./cmd/server`
2. Start client:
   - `go run ./cmd/client`

Optional manual UI harness (spawns multiple clients automatically):
- PowerShell:
  - `$env:PRISON_MANUAL_UI_TEST='true'`
  - `$env:PRISON_MANUAL_UI_TEST_CLIENTS='5'`
  - `go run ./cmd/client`

## 2. Core objective

- Authorities:
  - Control the prison, contain prisoners, and prevent escape outcomes.
- Prisoners:
  - Gather resources, use tasks/market/tools, and complete escape or faction win paths.

## 3. Match flow

- The game alternates Day and Night in real time.
- Many systems are phase-gated:
  - Black market is Night only.
  - NPC prisoner money tasks are Day focused.
  - Some abilities have per-day limits.

## 4. Controls

- Movement: `WASD` or Arrow keys
- Sprint: `Shift`
- Interact: `E` / `F`
- Shoot: `Space` or Left Mouse
- Reload: `R`
- Ability use: `V`
- Ability + role info panel: `I`
- Pause menu: `Esc` or `P`

Panels and modal controls:
- Inventory panel: `Tab`
- Cards panel: `C`
- Stash panel: `H` (in your cell block)
- Escape panel: `X`
- Market access hint: `B` / `M`
- Modal navigation: Arrow keys / panel next-prev keys
- Confirm in modal: `Enter`

## 5. Black market (important)

- To buy:
  1. Be a prisoner.
  2. Wait for Night.
  3. Go to the **current nightly market room**.
  4. Press `E`/`F` to open market modal.
  5. Select offer and press `Enter`.

Notes:
- The market room rotates nightly.
- It is not always the physical room named "Black Market".
- If you press interact in the static Black Market room when tonight's market moved elsewhere, the HUD now shows the correct room to go to.

## 6. Money and economy

- Money is represented by `Money` cards.
- Main earn path:
  - Interact with NPC prisoners during Day to get tasks.
  - Complete task and interact again to claim Money card rewards.
- Spend Money cards in the Night black market.

## 7. Inventory / cards / stash

- Inventory holds items used for combat, crafting, and escapes.
- Cards are tactical one-time effects.
- Cell stash:
  - Only your assigned cell stash can be used.
  - Use `H` in cell block to deposit/withdraw.

## 8. Ability usage rules

- Each player has an assigned ability.
- Press `V` to use ability directly.
- Press `I` to see your role + ability details and usage guidance.
- Some abilities are context-sensitive (room, phase, target, daily use limits).

## 9. If an action “does nothing”

Check these first:
- You are in the required room.
- Current phase matches the action (Day/Night).
- You have required resources (cards/items/ammo).
- Action is not on cooldown or daily limit.
- Read latest `Hint:` / `Event:` line in HUD for failure reason.
