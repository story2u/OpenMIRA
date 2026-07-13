import Image from 'next/image'
import { cn } from '@/lib/utils'

export function BrandLogo({
  size = 36,
  className,
  priority = false,
}: {
  size?: number
  className?: string
  priority?: boolean
}) {
  const source = size <= 64 ? '/logo-64.png' : '/logo-512.png'

  return (
    <Image
      src={source}
      width={size}
      height={size}
      alt=""
      aria-hidden="true"
      priority={priority}
      className={cn('shrink-0 rounded-[22%]', className)}
    />
  )
}
