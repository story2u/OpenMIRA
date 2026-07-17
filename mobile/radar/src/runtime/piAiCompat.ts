import { Compile } from 'typebox/compile';
import { Value } from 'typebox/value';

/**
 * React Native runtime surface required by pi-agent-core.
 *
 * pi-agent-core 0.80.6 imports the legacy pi-ai compatibility barrel even when
 * an app injects its own stream function. That barrel eagerly reaches Node-only
 * provider/auth modules, so Metro aliases it to this deliberately small surface.
 */
export class EventStream<T, R = T> implements AsyncIterable<T> {
  private queue: T[] = [];
  private waiting: Array<(value: IteratorResult<T>) => void> = [];
  private done = false;
  private readonly finalResult: Promise<R>;
  private resolveFinalResult!: (result: R) => void;

  constructor(
    private readonly isComplete: (event: T) => boolean,
    private readonly extractResult: (event: T) => R,
  ) {
    this.finalResult = new Promise<R>((resolve) => {
      this.resolveFinalResult = resolve;
    });
  }

  push(event: T) {
    if (this.done) return;
    if (this.isComplete(event)) {
      this.done = true;
      this.resolveFinalResult(this.extractResult(event));
    }
    const waiter = this.waiting.shift();
    if (waiter) waiter({ value: event, done: false });
    else this.queue.push(event);
  }

  end(result?: R) {
    this.done = true;
    if (result !== undefined) this.resolveFinalResult(result);
    for (const waiter of this.waiting.splice(0)) waiter({ value: undefined, done: true });
  }

  async *[Symbol.asyncIterator](): AsyncIterator<T> {
    while (true) {
      const queued = this.queue.shift();
      if (queued !== undefined) {
        yield queued;
      } else if (this.done) {
        return;
      } else {
        const value = await new Promise<IteratorResult<T>>((resolve) => this.waiting.push(resolve));
        if (value.done) return;
        yield value.value;
      }
    }
  }

  result() {
    return this.finalResult;
  }
}

interface ToolLike {
  name: string;
  parameters: Parameters<typeof Compile>[0];
}

interface ToolCallLike {
  name: string;
  arguments: unknown;
}

export function validateToolArguments(tool: ToolLike, toolCall: ToolCallLike) {
  const args = structuredClone(toolCall.arguments);
  Value.Convert(tool.parameters, args);
  const validator = Compile(tool.parameters);
  if (validator.Check(args)) return args;

  const errors = [...validator.Errors(args)].map((error) => error.message).join('; ');
  throw new Error(`Validation failed for tool "${tool.name}": ${errors}`);
}

export function streamSimple(): never {
  throw new Error('A React Native pi streamFn must be injected explicitly');
}
