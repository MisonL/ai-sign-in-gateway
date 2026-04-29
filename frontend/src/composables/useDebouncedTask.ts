import { onBeforeUnmount } from 'vue'

export function useDebouncedTask(task: () => Promise<void> | void, delay = 320) {
  let timer: number | null = null

  function cancel() {
    if (timer !== null) {
      window.clearTimeout(timer)
      timer = null
    }
  }

  function schedule() {
    cancel()
    timer = window.setTimeout(async () => {
      timer = null
      await task()
    }, delay)
  }

  onBeforeUnmount(cancel)

  return {
    schedule,
    cancel,
  }
}
