import { controlModifiedInput, mobileTerminalKeyBytes } from './mobile-terminal-keys';

describe('mobile terminal helper keys', () => {
  it('encodes terminal navigation keys as their real escape sequences', () => {
    expect([...mobileTerminalKeyBytes('escape')]).toEqual([27]);
    expect([...mobileTerminalKeyBytes('tab')]).toEqual([9]);
    expect([...mobileTerminalKeyBytes('arrow-left')]).toEqual([27, 91, 68]);
    expect([...mobileTerminalKeyBytes('arrow-up')]).toEqual([27, 91, 65]);
    expect([...mobileTerminalKeyBytes('arrow-down')]).toEqual([27, 91, 66]);
    expect([...mobileTerminalKeyBytes('arrow-right')]).toEqual([27, 91, 67]);
  });

  it('turns the next typed ASCII letter into its Ctrl terminal byte', () => {
    expect(controlModifiedInput('c')).toBe('\u0003');
    expect(controlModifiedInput('Z')).toBe('\u001a');
  });

  it('leaves a non-letter unchanged so it is never silently lost', () => {
    expect(controlModifiedInput('🙂')).toBe('🙂');
  });
});
