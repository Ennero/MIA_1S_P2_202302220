
import { createRouter, createWebHistory } from 'vue-router';

import inicio from '@/views/StartPage.vue';

import login from '@/views/LoginPage.vue';
import disk from '@/views/DiskPage.vue';
import loged from '@/views/LogedPage.vue';
import partitions from '@/views/PartitionsPage.vue';
import FilesPage from '@/views/FilesPage.vue';

const routes = [
    {
        path: '/', // La ruta raíz mostrará la consola
        name: 'inicio',
        component: inicio
    },
    {
        path: '/login', // La URL para tu nueva ventana/vista
        name: 'login',
        component: login
        // Puedes añadir más rutas aquí
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
    }


];

const router = createRouter({
    history: createWebHistory(process.env.BASE_URL),
    routes
});

export default router;