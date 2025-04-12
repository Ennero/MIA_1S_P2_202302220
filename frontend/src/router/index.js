
import { createRouter, createWebHistory } from 'vue-router';

import inicio from '@/views/StartPage.vue';

import login from '@/views/LoginPage.vue';
import disk from '@/views/DiskPage.vue';
import loged from '@/views/LogedPage.vue';
import partitions from '@/views/PartitionsPage.vue';
import FilesPage from '@/views/FilesPage.vue';
import FileView from '@/views/FileView.vue';

const routes = [
    {
        path: '/', 
        name: 'inicio',
        component: inicio
    },
    {
        path: '/login', 
        name: 'login',
        component: login
    },
    {
        path: '/disk',
        name: 'disk',
        component: disk
    },
    {
        path: '/loged',
        name: 'loged',
        component: loged
    },
    {
        path: '/partitions/:diskPathEncoded',
        name: 'partitions',
        component: partitions,
        props: true
    },
    {
        path: '/FilesPage/:mountId/:internalPathEncoded(.*)',
        name: 'FilesPage',
        component: FilesPage,
        props: true // Pasa mountId e internalPathEncoded como props
    },
    {
        path: '/FilesPage/:mountId',
        redirect: to => {
            // %2F es '/' codificado para URL
            return { path: `/FilesPage/${to.params.mountId}/%2F` }
        }
    },
    {
        path: '/view/:mountId/:filePathEncoded(.*)', // Captura cualquier ruta después de /view/
        name: 'FileView',
        component: FileView,
        props: true // Pasa mountId e internalPathEncoded como props
    }



];

const router = createRouter({
    history: createWebHistory(process.env.BASE_URL),
    routes
});

export default router;