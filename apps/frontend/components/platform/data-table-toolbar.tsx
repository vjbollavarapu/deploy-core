import type { ReactNode } from 'react'
import { FilterBar } from './filter-bar'
import { SearchInput } from './search-input'

interface DataTableToolbarProps {
  searchPlaceholder?: string
  searchValue?: string
  onSearchChange?: (value: string) => void
  filters?: ReactNode
  actions?: ReactNode
  className?: string
}

export function DataTableToolbar({
  searchPlaceholder = 'Search…',
  searchValue,
  onSearchChange,
  filters,
  actions,
  className,
}: DataTableToolbarProps) {
  return (
    <FilterBar className={className} end={actions}>
      {onSearchChange !== undefined && (
        <SearchInput
          containerClassName="w-full sm:max-w-xs"
          placeholder={searchPlaceholder}
          value={searchValue}
          onChange={(e) => onSearchChange(e.target.value)}
          aria-label={searchPlaceholder}
        />
      )}
      {filters}
    </FilterBar>
  )
}
