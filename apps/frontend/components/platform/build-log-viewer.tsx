'use client'

import {
  Check,
  Copy,
  Download,
  Maximize2,
  Minimize2,
  Pause,
  Play,
  Search,
} from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { InputGroup, InputGroupAddon, InputGroupInput } from '@/components/ui/input-group'
import { Toggle } from '@/components/ui/toggle'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { LOG_LEVELS } from '@/lib/observability'
import type { LogLine } from '@/lib/types'

interface BuildLogViewerProps {
  lines: LogLine[]
  className?: string
  /** When true, shows a live streaming indicator (UI-ready; no network). */
  streaming?: boolean
  title?: string
  /** Show severity toggles (default true for observability viewers). */
  showSeverity?: boolean
}

const LEVEL_CLASSES: Record<LogLine['level'], string> = {
  info: 'text-foreground',
  debug: 'text-muted-foreground',
  warn: 'text-warning',
  error: 'text-critical',
}

const ROW_HEIGHT = 28
const OVERSCAN = 12
const VIRTUALIZE_THRESHOLD = 80
const DEFAULT_LEVELS = [...LOG_LEVELS]

export function BuildLogViewer({
  lines,
  className,
  streaming = true,
  title = 'Build logs',
  showSeverity = true,
}: BuildLogViewerProps) {
  const [query, setQuery] = useState('')
  const [levels, setLevels] = useState<string[]>(DEFAULT_LEVELS)
  const [showTimestamps, setShowTimestamps] = useState(true)
  const [following, setFollowing] = useState(true)
  const [paused, setPaused] = useState(false)
  const [fullscreen, setFullscreen] = useState(false)
  const [copied, setCopied] = useState(false)
  const [scrollTop, setScrollTop] = useState(0)
  const [viewportHeight, setViewportHeight] = useState(420)

  const scrollRef = useRef<HTMLDivElement>(null)
  const live = streaming && !paused

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    const activeLevels = levels.length > 0 ? levels : DEFAULT_LEVELS
    return lines.filter((line) => {
      if (!activeLevels.includes(line.level)) return false
      if (!q) return true
      return (
        line.message.toLowerCase().includes(q) ||
        line.level.includes(q) ||
        line.container.toLowerCase().includes(q) ||
        (line.application?.toLowerCase().includes(q) ?? false) ||
        (line.revision?.toLowerCase().includes(q) ?? false)
      )
    })
  }, [lines, query, levels])

  const useVirtual = filtered.length >= VIRTUALIZE_THRESHOLD

  const { start, end, offsetY, totalHeight } = useMemo(() => {
    if (!useVirtual) {
      return { start: 0, end: filtered.length, offsetY: 0, totalHeight: filtered.length * ROW_HEIGHT }
    }
    const startIndex = Math.max(0, Math.floor(scrollTop / ROW_HEIGHT) - OVERSCAN)
    const visible = Math.ceil(viewportHeight / ROW_HEIGHT) + OVERSCAN * 2
    const endIndex = Math.min(filtered.length, startIndex + visible)
    return {
      start: startIndex,
      end: endIndex,
      offsetY: startIndex * ROW_HEIGHT,
      totalHeight: filtered.length * ROW_HEIGHT,
    }
  }, [filtered.length, scrollTop, useVirtual, viewportHeight])

  const visibleLines = filtered.slice(start, end)

  useEffect(() => {
    const el = scrollRef.current
    if (!el) return
    const update = () => setViewportHeight(el.clientHeight)
    update()
    const observer = new ResizeObserver(update)
    observer.observe(el)
    return () => observer.disconnect()
  }, [fullscreen])

  useEffect(() => {
    if (!following || paused) return
    const el = scrollRef.current
    if (!el) return
    el.scrollTop = el.scrollHeight
  }, [filtered.length, following, paused, fullscreen])

  const onScroll = useCallback(() => {
    const el = scrollRef.current
    if (!el) return
    setScrollTop(el.scrollTop)
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 40
    if (!atBottom && following) setFollowing(false)
  }, [following])

  const textBlob = useMemo(
    () =>
      filtered
        .map((line) => {
          const prefix = showTimestamps ? `${line.timestamp} ` : ''
          const meta = [line.application, line.environment, line.revision]
            .filter(Boolean)
            .join('/')
          const metaPart = meta ? ` ${meta}` : ''
          return `${prefix}[${line.level}] ${line.container}${metaPart} ${line.message}`
        })
        .join('\n'),
    [filtered, showTimestamps],
  )

  async function handleCopy() {
    await navigator.clipboard.writeText(textBlob)
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }

  function handleDownload() {
    const blob = new Blob([textBlob], { type: 'text/plain;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `${title.replace(/\s+/g, '-').toLowerCase()}.log`
    a.click()
    URL.revokeObjectURL(url)
  }

  return (
    <div
      className={cn(
        'flex flex-col overflow-hidden rounded-lg border border-border bg-card',
        fullscreen && 'fixed inset-3 z-50 shadow-lg',
        className,
      )}
    >
      <div className="flex flex-wrap items-center gap-2 border-b border-border p-2.5">
        <InputGroup className="max-w-xs">
          <InputGroupInput
            placeholder="Search logs..."
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
          <InputGroupAddon>
            <Search />
          </InputGroupAddon>
        </InputGroup>
        {showSeverity ? (
          <ToggleGroup
            value={levels}
            onValueChange={(next) => setLevels(next.length > 0 ? next : DEFAULT_LEVELS)}
            size="sm"
            variant="outline"
          >
            {LOG_LEVELS.map((level) => (
              <ToggleGroupItem key={level} value={level}>
                {level}
              </ToggleGroupItem>
            ))}
          </ToggleGroup>
        ) : null}
        <Toggle
          pressed={showTimestamps}
          onPressedChange={setShowTimestamps}
          size="sm"
          variant="outline"
          aria-label="Toggle timestamps"
        >
          Timestamps
        </Toggle>
        <Toggle
          pressed={following}
          onPressedChange={(v) => {
            setFollowing(v)
            if (v) setPaused(false)
          }}
          size="sm"
          variant="outline"
          aria-label="Follow log output"
        >
          Follow
        </Toggle>
        <div className="ml-auto flex items-center gap-1.5">
          <Button variant="outline" size="sm" onClick={() => setPaused((v) => !v)}>
            {paused ? <Play data-icon="inline-start" /> : <Pause data-icon="inline-start" />}
            {paused ? 'Resume' : 'Pause'}
          </Button>
          <Button variant="outline" size="sm" onClick={handleCopy}>
            {copied ? <Check data-icon="inline-start" /> : <Copy data-icon="inline-start" />}
            {copied ? 'Copied' : 'Copy'}
          </Button>
          <Button variant="outline" size="sm" onClick={handleDownload}>
            <Download data-icon="inline-start" />
            Download
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setFullscreen((v) => !v)}
            aria-label={fullscreen ? 'Exit fullscreen' : 'Enter fullscreen'}
          >
            {fullscreen ? (
              <Minimize2 data-icon="inline-start" />
            ) : (
              <Maximize2 data-icon="inline-start" />
            )}
            {fullscreen ? 'Exit' : 'Fullscreen'}
          </Button>
        </div>
      </div>

      <div
        ref={scrollRef}
        onScroll={onScroll}
        className={cn(
          'overflow-y-auto bg-background/40 font-mono text-xs leading-relaxed',
          fullscreen ? 'flex-1' : 'max-h-[520px]',
        )}
      >
        {filtered.length === 0 ? (
          <div className="p-6 text-center text-sm text-muted-foreground">
            No log lines match your search.
          </div>
        ) : useVirtual ? (
          <div style={{ height: totalHeight, position: 'relative' }}>
            <div style={{ transform: `translateY(${offsetY}px)` }}>
              {visibleLines.map((line) => (
                <LogRow key={line.id} line={line} showTimestamps={showTimestamps} />
              ))}
            </div>
          </div>
        ) : (
          visibleLines.map((line) => (
            <LogRow key={line.id} line={line} showTimestamps={showTimestamps} />
          ))
        )}
        {live && (
          <div className="flex items-center gap-2 px-3 py-2 text-muted-foreground">
            <span className="relative flex size-1.5">
              <span className="absolute inset-0 rounded-full bg-success" />
              <span className="absolute inset-0 animate-ping rounded-full bg-success opacity-60" />
            </span>
            Streaming live
          </div>
        )}
        {paused && (
          <div className="px-3 py-2 text-xs text-warning">Paused — new lines will not auto-scroll</div>
        )}
      </div>
    </div>
  )
}

function LogRow({ line, showTimestamps }: { line: LogLine; showTimestamps: boolean }) {
  return (
    <div
      className="flex gap-3 border-b border-border/50 px-3 hover:bg-muted/40"
      style={{ height: ROW_HEIGHT, alignItems: 'center' }}
    >
      {showTimestamps ? (
        <span className="shrink-0 tabular text-muted-foreground">{line.timestamp}</span>
      ) : null}
      <span className={cn('w-12 shrink-0 uppercase', LEVEL_CLASSES[line.level])}>{line.level}</span>
      <span className="shrink-0 text-muted-foreground">{line.container}</span>
      {line.application ? (
        <span className="hidden shrink-0 text-muted-foreground sm:inline">{line.application}</span>
      ) : null}
      <span className="min-w-0 flex-1 truncate text-foreground">{line.message}</span>
    </div>
  )
}
