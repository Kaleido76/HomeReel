// Playback timing constants shared by the player and the series member list:
// progress within the last RESUME_TAIL seconds is treated as "finished" (the
// player starts fresh, the member list hides the resume bar), and anything under
// RESUME_MIN is too little to bother resuming.
export const RESUME_MIN = 10
export const RESUME_TAIL = 20
// Progress persistence cadence (ADR plan: ~10s).
export const SAVE_INTERVAL = 10
// Within this many seconds of the end the player stops persisting progress.
export const NEAR_END = 1
