<template>
  <section>
    <MainLayout :show-sidebar="showSidebar">
      <template
        v-if="showSidebar"
        #sidebar
      >
        <SideBar />
      </template>
      <template #main>
        <MainContainer />
      </template>
    </MainLayout>
  </section>
</template>

<script setup lang="ts">
import MainLayout from '@/layout/main-layout/index.vue'
import SideBar from '@/components/sidebar/index.vue'
import MainContainer from '@/components/main-container/index.vue'
import { computed } from 'vue'
import { useRoute } from 'vue-router'

const route = useRoute()
const showSidebar = computed(() => route.name === 'chat')
</script>
