'use client'

import { useCallback, useEffect, useReducer, useState } from 'react'
import { ApiError } from '@/lib/api'

export interface QueryState<T> {
  data: T | null
  isLoading: boolean
  error: string | null
  errorCode: string | null
  reload: () => void
}

type Action<T> =
  | { type: 'loading' }
  | { type: 'success'; data: T }
  | { type: 'error'; message: string; code: string | null }

function reducer<T>(state: QueryState<T>, action: Action<T>): QueryState<T> {
  switch (action.type) {
    case 'loading':
      return { ...state, isLoading: true, error: null, errorCode: null }
    case 'success':
      return { ...state, isLoading: false, data: action.data, error: null, errorCode: null }
    case 'error':
      return { ...state, isLoading: false, error: action.message, errorCode: action.code }
    default:
      return state
  }
}

/**
 * Minimal fetch hook providing loading, success, empty, and error states.
 *
 * @param fetcher - async function that returns data; recreate with useCallback to re-fetch
 */
export function useApiQuery<T>(fetcher: () => Promise<T>): QueryState<T> {
  const [runKey, setRunKey] = useState(0)
  const [state, dispatch] = useReducer(reducer<T>, {
    data: null,
    isLoading: true,
    error: null,
    errorCode: null,
    reload: () => {},
  })

  useEffect(() => {
    let cancelled = false

    async function run() {
      await Promise.resolve()
      if (cancelled) return
      dispatch({ type: 'loading' })

      try {
        const result = await fetcher()
        if (!cancelled) {
          dispatch({ type: 'success', data: result })
        }
      } catch (err) {
        if (cancelled) return
        if (err instanceof ApiError) {
          dispatch({ type: 'error', message: err.message, code: err.code })
        } else {
          dispatch({
            type: 'error',
            message: err instanceof Error ? err.message : 'An unexpected error occurred',
            code: null,
          })
        }
      }
    }

    void run()

    return () => {
      cancelled = true
    }
    // runKey is the only stable dep; fetcher identity triggers via parent useCallback
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [runKey])

  const reload = useCallback(() => {
    setRunKey((k) => k + 1)
  }, [])

  return { ...state, reload }
}
