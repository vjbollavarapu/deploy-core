'use client'

import { MoreHorizontal, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { DestructiveConfirmDialog } from '@/components/platform/destructive-confirm-dialog'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { CopyButton } from '@/components/platform/copy-button'
import type { ContainerImage } from '@/lib/types'

interface ImagesTableProps {
  images: ContainerImage[]
}

export function ImagesTable({ images }: ImagesTableProps) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Repository</TableHead>
          <TableHead>Tag</TableHead>
          <TableHead>Digest</TableHead>
          <TableHead>Size</TableHead>
          <TableHead>Application</TableHead>
          <TableHead>Created</TableHead>
          <TableHead className="w-10 text-right">
            <span className="sr-only">Actions</span>
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {images.map((img) => (
          <TableRow key={img.id}>
            <TableCell>
              <div className="flex flex-col gap-0.5">
                <span className="font-mono text-sm text-foreground">{img.name}</span>
                <span className="font-mono text-[11px] text-muted-foreground">{img.location}</span>
              </div>
            </TableCell>
            <TableCell className="font-mono text-xs">{img.tag}</TableCell>
            <TableCell>
              <div className="flex items-center gap-1">
                <span className="max-w-40 truncate font-mono text-xs text-muted-foreground" title={img.digest}>
                  {img.digest}
                </span>
                <CopyButton value={img.digest} label="Copy digest" />
              </div>
            </TableCell>
            <TableCell className="tabular text-sm">{img.sizeMb} MB</TableCell>
            <TableCell className="text-sm">{img.application}</TableCell>
            <TableCell className="text-xs text-muted-foreground">{img.createdAt}</TableCell>
            <TableCell className="text-right">
              <ImageRowActions image={img} />
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

function ImageRowActions({ image }: { image: ContainerImage }) {
  const [removeOpen, setRemoveOpen] = useState(false)
  const ref = `${image.name}:${image.tag}`

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button variant="ghost" size="icon-sm" aria-label={`Actions for ${ref}`} />
          }
        >
          <MoreHorizontal />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem variant="destructive" onClick={() => setRemoveOpen(true)}>
            <Trash2 />
            Delete image
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <DestructiveConfirmDialog
        open={removeOpen}
        onOpenChange={setRemoveOpen}
        title={`Delete image ${ref}?`}
        description="This removes the image from the registry view. Running containers that already pulled it are not affected."
        confirmLabel="Delete image"
        confirmationPhrase={ref}
        onConfirm={() => {
          toast.success(`${ref} deleted`)
        }}
      />
    </>
  )
}
