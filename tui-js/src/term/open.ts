type LoadingStateConsumer = (label: string | null) => void;

export class TerminalOpenCoordinator {
  private running = false;
  private blockedUntil = 0;

  constructor(
    private readonly setLoading: LoadingStateConsumer,
    private readonly cooldownMilliseconds = 300,
  ) {}

  async run(label: string, action: () => Promise<void>): Promise<boolean> {
    if (this.running || Date.now() < this.blockedUntil) return false;
    this.running = true;
    this.setLoading(label);
    try {
      await action();
      return true;
    } finally {
      this.setLoading(null);
      this.running = false;
      this.blockedUntil = Date.now() + this.cooldownMilliseconds;
    }
  }
}
