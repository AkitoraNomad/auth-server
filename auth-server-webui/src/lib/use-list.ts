import { useEffect, useReducer, useState } from 'react'
import { errorMessage } from './api'

// Callers supply a stable loader; each reload invalidates the previous result.
export function useList<T>(load: () => Promise<T[]>) {
  const [data, setData] = useState<T[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [version, reload] = useReducer((value: number) => value + 1, 0)

  useEffect(() => {
    let active = true
    load().then((items) => {
      if (active) setData(items)
    }).catch((err: unknown) => {
      if (active) setError(errorMessage(err))
    }).finally(() => {
      if (active) setLoading(false)
    })
    return () => { active = false }
  }, [load, version])

  const refresh = () => {
    setLoading(true)
    setError(null)
    reload()
  }
  return { data, loading, error, setError, reload: refresh }
}
