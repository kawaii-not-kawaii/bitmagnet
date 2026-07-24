export type SwarmTier = "healthy" | "fair" | "weak" | "unknown";

export interface SwarmHealth {
  tier: SwarmTier;
  /** Lit bars of the three-bar glyph; 0 when the seeder count is unknown. */
  litBars: number;
  /** Tier colour, or empty when unknown so the row inherits its normal colour. */
  color: string;
  tooltip: string;
}

export const SWARM_BARS = 3;

// Fixed hex rather than theme variables: red/amber/green carry meaning that must
// survive all 20 themes.
const TIER_COLORS: Record<Exclude<SwarmTier, "unknown">, string> = {
  healthy: "#3fb950",
  fair: "#d29922",
  weak: "#e5484d",
};

const TIER_BARS: Record<Exclude<SwarmTier, "unknown">, number> = {
  healthy: 3,
  fair: 2,
  weak: 1,
};

/**
 * Grades a swarm by seeder count. An unknown count is not a bad swarm — it maps
 * to its own tier rather than falling through to `weak`.
 */
export function swarmHealth(
  seeders: number | null | undefined,
  seedHealthy: number,
  seedFair: number,
): SwarmHealth {
  if (seeders === null || seeders === undefined || !Number.isFinite(seeders)) {
    return {
      tier: "unknown",
      litBars: 0,
      color: "",
      tooltip: "unknown swarm",
    };
  }

  const tier: Exclude<SwarmTier, "unknown"> =
    seeders >= seedHealthy ? "healthy" : seeders >= seedFair ? "fair" : "weak";

  return {
    tier,
    litBars: TIER_BARS[tier],
    color: TIER_COLORS[tier],
    tooltip: `${TIER_BARS[tier]}/${SWARM_BARS} · ${tier} swarm`,
  };
}
