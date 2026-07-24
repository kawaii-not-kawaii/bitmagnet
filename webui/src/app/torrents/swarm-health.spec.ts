import { swarmHealth } from "./swarm-health";

describe("swarmHealth", () => {
  const healthy = 500;
  const fair = 50;

  it("grades a well-seeded swarm as healthy", () => {
    const result = swarmHealth(1200, healthy, fair);
    expect(result.tier).toBe("healthy");
    expect(result.litBars).toBe(3);
    expect(result.color).toBe("#3fb950");
    expect(result.tooltip).toBe("3/3 · healthy swarm");
  });

  it("treats the healthy threshold as inclusive", () => {
    expect(swarmHealth(healthy, healthy, fair).tier).toBe("healthy");
  });

  it("grades a mid-range swarm as fair", () => {
    const result = swarmHealth(120, healthy, fair);
    expect(result.tier).toBe("fair");
    expect(result.litBars).toBe(2);
    expect(result.tooltip).toBe("2/3 · fair swarm");
  });

  it("treats the fair threshold as inclusive", () => {
    expect(swarmHealth(fair, healthy, fair).tier).toBe("fair");
  });

  it("grades a poorly-seeded swarm as weak", () => {
    const result = swarmHealth(fair - 1, healthy, fair);
    expect(result.tier).toBe("weak");
    expect(result.litBars).toBe(1);
  });

  it("grades zero seeders as weak, not unknown", () => {
    expect(swarmHealth(0, healthy, fair).tier).toBe("weak");
  });

  it("does not grade an unknown count as weak", () => {
    for (const value of [null, undefined]) {
      const result = swarmHealth(value, healthy, fair);
      expect(result.tier).toBe("unknown");
      expect(result.litBars).toBe(0);
      expect(result.color).toBe("");
    }
  });

  it("retiers when the thresholds move", () => {
    // The handoff expects seedHealthy to fall toward ~200 against live data.
    expect(swarmHealth(300, 500, 50).tier).toBe("fair");
    expect(swarmHealth(300, 200, 50).tier).toBe("healthy");
  });
});
