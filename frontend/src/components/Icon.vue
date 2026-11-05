<script setup>
defineProps({ name: String, size: { type: Number, default: 20 } });
const paths = {
  lock: 'M5 10h14v11H5zM8 10V6a4 4 0 0 1 8 0v4M12 14v3',
  logout: 'M9 4H4v16h5M12 12h9m-4-4 4 4-4 4',
  eye: 'M2 12s4-7 10-7 10 7 10 7-4 7-10 7-10-7-10-7ZM15 12a3 3 0 1 1-6 0 3 3 0 0 1 6 0',
  eyeOff:
    'M3 3l18 18M10 5h2c6 0 10 7 10 7a18 18 0 0 1-3 4M6 6a20 20 0 0 0-4 6s4 7 10 7a12 12 0 0 0 5-1M10 10a3 3 0 0 0 4 4',
  wind: 'M3 8h12a3 3 0 1 0-3-3M3 12h18M3 16h9a3 3 0 1 1-3 3',
  monitor: 'M3 4h18v13H3zM8 21h8M12 17v4M6 12l3-3 3 3 5-5',
  bell: 'M18 8a6 6 0 0 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9M10 21h4',
  grid: 'M3 3h7v7H3zM14 3h7v7h-7zM3 14h7v7H3zM14 14h7v7h-7z',
  data: 'M4 4h16v16H4zM4 9h16M4 14h16M10 4v16',
  settings: 'M4 7h16M4 17h16M8 4v6M16 14v6',
  plus: 'M12 5v14M5 12h14',
  check: 'M5 12l4 4L19 6',
  chevron: 'M9 5l7 7-7 7',
  down: 'M6 9l6 6 6-6',
  back: 'M19 12H5m6-6-6 6 6 6',
  close: 'M6 6l12 12M6 18 18 6',
  search: 'M21 21l-6-6M17 10a7 7 0 1 1-14 0 7 7 0 0 1 14 0',
  code: 'M8 5 2 12l6 7M16 5l6 7-6 7M14 3l-4 18',
  globe: 'M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0M3 12h18M12 3c5 5 5 13 0 18-5-5-5-13 0-18',
  rss: 'M4 11a9 9 0 0 1 9 9M4 4a16 16 0 0 1 16 16M4 19h1v1H4z',
  play: 'm8 5 11 7-11 7z',
  pause: 'M8 5v14M16 5v14',
  clock: 'M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0M12 7v5l3 2',
  people:
    'M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2M13 3a4 4 0 1 1 0 8M22 21v-2a4 4 0 0 0-4-4M12 7a4 4 0 1 1-8 0 4 4 0 0 1 8 0',
  user: 'M20 21v-2a6 6 0 0 0-6-6h-4a6 6 0 0 0-6 6v2M16 6a4 4 0 1 1-8 0 4 4 0 0 1 8 0',
  send: 'm22 2-7 20-4-9-9-4 20-7ZM22 2 11 13',
  link: 'm10 13 4-4M8 16l-2 2a4 4 0 0 1-5-5l5-5a4 4 0 0 1 5 0M16 8l2-2a4 4 0 0 1 5 5l-5 5a4 4 0 0 1-5 0',
  arrow: 'M5 12h14m-6-6 6 6-6 6',
  mail: 'M3 5h18v14H3zM3 5l9 7 9-7',
  info: 'M12 11v6M12 7v1M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0',
  refresh: 'M20 7v5h-5M4 17v-5h5M6 6a8 8 0 0 1 14 6M18 18a8 8 0 0 1-14-6',
  more: 'M5 11v2M12 11v2M19 11v2',
  edit: 'm16 3 5 5-12 12-6 1 1-6L16 3ZM13 6l5 5',
  trash: 'M3 6h18M9 6V3h6v3M5 6l1 15h12l1-15M10 10v7M14 10v7',
  shield: 'M12 3 3 7v5c0 5 9 9 9 9s9-4 9-9V7l-9-4ZM8 12l3 3 5-6',
  spark: 'm12 3 3 6 6 3-6 3-3 6-3-6-6-3 6-3z',
  folder: 'M3 5h7l2 3h9v13H3z'
};
</script>
<template>
  <svg
    :width="size"
    :height="size"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width="1.7"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
  >
    <path :d="paths[name] || paths.grid" />
  </svg>
</template>
