import type { InfoBroker } from '../contracts.ts';

// No data sources: requester text passes through, every request is not available.
export class NullBroker implements InfoBroker {
  async onRequesterText(_conv: string, _requester: string, text: string): Promise<{ text: string }> {
    return { text };
  }
  async request(): Promise<{ notAvailable: string }> {
    return { notAvailable: 'no data sources are configured' };
  }
  subscribe(): () => void {
    return () => {};
  }
  async forget(): Promise<void> {}
  async instructions(): Promise<null> {
    return null;
  }
}
